package node

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	"github.com/gosuri/uilive"
	"github.com/libp2p/go-libp2p"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	rcmgr "github.com/libp2p/go-libp2p/p2p/host/resource-manager"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-varint"
	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/crypt"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
	"github.com/dennis-tra/pcp/pkg/service"
	"github.com/dennis-tra/pcp/pkg/words"
)

// State represents the state the node is in. Most importantly, if the
// node switches to the Connected state all advertisement/discovery
// tasks stop, and it doesn't accept new peers to connect.
type State string

const (
	// Initialising means the node has booted but not yet
	// started to advertise/discover the word list/other peer.
	Initialising State = "initialising"
	// Roaming means the node is actively advertising/discovering
	// the word list/other peers
	Roaming = "roaming"
	// Connected means the node has found the corresponding peer
	// and successfully authenticated it.
	Connected = "connected"
)

// Node encapsulates the logic that's common for the receiving
// and sending side of the file transfer.
type Node struct {
	service.Service
	host.Host

	// if verbose logging is activated
	Verbose bool

	// give node protocol capabilities
	PushProtocol
	TransferProtocol
	PakeProtocol

	// keeps track of hole punching states of particular peers.
	// The hpAllowList is populated by the receiving side of
	// the file transfer after it has discovered the peer via
	// mDNS or in the DHT. If a peer is in the hpAllowList the
	// hpStates map will track the hole punching state for
	// that particular peer. The sending side doesn't work
	// with that map and instead tracks all **incoming** hole
	// punches. I've observed that the sending side does try
	// to hole punch peers it finds in the DHT (while advertising).
	// These are hole punches we're not interested in.
	hpStatesLk  sync.RWMutex
	hpStates    map[peer.ID]*HolePunchState
	hpAllowList sync.Map

	// DHT is an accessor that is needed in the DHT discoverer/advertiser.
	DHT        *kaddht.IpfsDHT
	NATManager basichost.NATManager

	ChanID int
	Words  []string

	stateLk *sync.RWMutex
	state   State
}

// New creates a new, fully initialized node with the given options.
func New(c *cli.Context, wrds []string, opts ...libp2p.Option) (*Node, error) {
	log.Debugln("Initialising local node...")

	if c.Bool("homebrew") {
		wrds = words.HomebrewList()
	}
	ints, err := words.ToInts(wrds)
	if err != nil {
		return nil, err
	}

	// set to something large, we're manually flushing the output
	uilive.RefreshInterval = 100 * time.Hour

	node := &Node{
		Service:  service.New("node"),
		hpStates: map[peer.ID]*HolePunchState{},
		state:    Initialising,
		stateLk:  &sync.RWMutex{},
		Words:    wrds,
		ChanID:   ints[0],
		Verbose:  c.Bool("verbose"),
	}
	node.PushProtocol = NewPushProtocol(node)
	node.TransferProtocol = NewTransferProtocol(node)
	node.PakeProtocol, err = NewPakeProtocol(node, wrds)
	if err != nil {
		return nil, fmt.Errorf("new pake protocol: %w", err)
	}

	// Configure the resource manager to not limit anything
	limiter := rcmgr.NewFixedLimiter(rcmgr.InfiniteLimits)
	rm, err := rcmgr.NewResourceManager(limiter)
	if err != nil {
		return nil, fmt.Errorf("new resource manager: %w", err)
	}

	opts = append(opts,
		libp2p.UserAgent("pcp/"+c.App.Version),
		libp2p.ResourceManager(rm),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			node.DHT, err = kaddht.New(c.Context, h, kaddht.EnableOptimisticProvide())
			return node.DHT, err
		}),
		libp2p.EnableHolePunching(holepunch.WithTracer(node)),
		libp2p.NATManager(func(network network.Network) basichost.NATManager {
			node.NATManager = basichost.NewNATManager(network)
			return node.NATManager
		}),
	)

	node.Host, err = libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("new libp2p host: %w", err)
	}

	log.Debugln("Initialized libp2p host:", node.ID().String())

	return node, node.ServiceStarted()
}

func (n *Node) SetState(s State) State {
	n.stateLk.Lock()
	log.Debugln("Setting local node state to", s)
	prevState := n.state
	n.state = s
	n.stateLk.Unlock()
	return prevState
}

func (n *Node) GetState() State {
	n.stateLk.RLock()
	defer n.stateLk.RUnlock()
	return n.state
}

// Send prepares the message msg to be sent over the network stream s.
// Send closes the stream for writing but leaves it open for reading.
func (n *Node) Send(s network.Stream, msg p2p.HeaderMessage) error {
	// Get own public key.
	pub, err := crypto.MarshalPublicKey(n.Host.Peerstore().PubKey(n.Host.ID()))
	if err != nil {
		return fmt.Errorf("marshal public key: %w", err)
	}

	hdr := &p2p.Header{
		RequestId:  uuid.New().String(),
		NodeId:     n.Host.ID().String(),
		NodePubKey: pub,
		Timestamp:  time.Now().Unix(),
	}
	msg.SetHeader(hdr)
	log.Debugf("Sending message %T to %s with request ID %s\n", msg, s.Conn().RemotePeer().String(), hdr.RequestId)

	// Transform msg to binary to calculate the signature.
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal proto message %T: %w", msg, err)
	}

	// Sign the data and attach the signature.
	key := n.Host.Peerstore().PrivKey(n.Host.ID())
	signature, err := key.Sign(data)
	if err != nil {
		return fmt.Errorf("sign message %T: %w", msg, err)
	}
	hdr.Signature = signature
	msg.SetHeader(hdr) // Maybe unnecessary

	// Transform msg + signature to binary.
	data, err = proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal signed proto message %T: %w", msg, err)
	}

	// Encrypt the data with the PAKE session key if it is found
	sKey, found := n.GetSessionKey(s.Conn().RemotePeer())
	if found {
		data, err = crypt.Encrypt(sKey, data)
		if err != nil {
			return fmt.Errorf("encrypt message %T: %w", msg, err)
		}
	}

	// Transmit the data.
	size := varint.ToUvarint(uint64(len(data)))
	_, err = s.Write(append(size, data...))
	if err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	return nil
}

// authenticateMessage verifies the authenticity of the message payload.
// It takes the given signature and verifies it against the given public
// key.
func (n *Node) authenticateMessage(msg p2p.HeaderMessage) (bool, error) {
	// store a temp ref to signature and remove it from message msg
	// sign is a string to allow easy reset to zero-value (empty string)
	signature := msg.GetHeader().Signature
	msg.GetHeader().Signature = nil

	// marshall msg without the signature to protobuf binary format
	bin, err := proto.Marshal(msg)
	if err != nil {
		return false, err
	}

	// restore sig in message msg (for possible future use)
	msg.GetHeader().Signature = signature

	// restore peer id binary format from base58 encoded node id msg
	peerID, err := peer.Decode(msg.GetHeader().NodeId)
	if err != nil {
		return false, fmt.Errorf("decode msg peer id: %w", err)
	}

	key, err := crypto.UnmarshalPublicKey(msg.GetHeader().NodePubKey)
	if err != nil {
		return false, fmt.Errorf("extract key from message key msg: %w", err)
	}

	// extract node id from the provided public key
	idFromKey, err := peer.IDFromPublicKey(key)
	if err != nil {
		return false, fmt.Errorf("extract peer id from public key: %w", err)
	}

	// verify that message author node id matches the provided node public key
	if idFromKey != peerID {
		return false, fmt.Errorf("node id and provided public key mismatch")
	}

	return key.Verify(bin, signature)
}

// Read drains the given stream and parses the content. It unmarshalls
// it into the protobuf object. It also verifies the authenticity of the message.
// Read closes the stream for reading but leaves it open for writing.
func (n *Node) Read(s network.Stream, buf p2p.HeaderMessage) error {
	data, err := n.ReadBytes(s)
	if err != nil {
		if err2 := s.Reset(); err2 != nil {
			err = fmt.Errorf("%s: %w", err2.Error(), err)
		}
		return err
	}

	log.Debugf("Reading message from %s\n", s.Conn().RemotePeer().String())
	// Decrypt the data with the PAKE session key if it is found
	sKey, found := n.GetSessionKey(s.Conn().RemotePeer())
	if found {
		data, err = crypt.Decrypt(sKey, data)
		if err != nil {
			return err
		}
	}

	if err = proto.Unmarshal(data, buf); err != nil {
		return fmt.Errorf("unmarshal received data: %w", err)
	}
	log.Debugf("type %T with request ID %s\n", buf, buf.GetHeader().RequestId)

	valid, err := n.authenticateMessage(buf)
	if err != nil {
		return fmt.Errorf("authenticate msg: %w", err)
	} else if !valid {
		return fmt.Errorf("failed to authenticate message")
	}

	return nil
}

// WriteBytes writes the given bytes to the destination writer and
// prefixes it with an uvarint indicating the length of the data.
func (n *Node) WriteBytes(w io.Writer, data []byte) (int, error) {
	size := varint.ToUvarint(uint64(len(data)))
	return w.Write(append(size, data...))
}

// byteReader turns an io.byteReader into an io.ByteReader.
type byteReader struct {
	io.Reader
}

func (r *byteReader) ReadByte() (byte, error) {
	var buf [1]byte
	n, err := r.Reader.Read(buf[:])
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, io.ErrNoProgress
	}
	return buf[0], nil
}

// ReadBytes reads an uvarint from the source reader to know how
// much data is following.
func (n *Node) ReadBytes(r io.Reader) ([]byte, error) {
	// bufio.NewReader wouldn't work because it would read the bytes
	// until its buffer (4096 bytes by default) is full. This data
	// isn't available outside of this function.
	br := &byteReader{r}
	l, err := varint.ReadUvarint(br)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, l)
	_, err = br.Read(buf)

	return buf, err
}

// ResetOnShutdown resets the given stream if the node receives a shutdown
// signal to indicate to our peer that we're not interested in the conversation
// anymore. Put the following at the top of a new stream handler:
//
//	defer n.ResetOnShutdown(stream)()
func (n *Node) ResetOnShutdown(s network.Stream) context.CancelFunc {
	cancel := make(chan struct{})
	go func() {
		select {
		case <-n.SigShutdown():
			s.Reset()
		case <-cancel:
		}
	}()
	return func() { close(cancel) }
}

// WaitForEOF waits for an EOF signal on the stream. This indicates that the peer
// has received all data and won't read from this stream anymore. Alternatively
// there is a 10-second timeout.
func (n *Node) WaitForEOF(s network.Stream) error {
	log.Debugln("Waiting for stream reset from peer...")
	timeout := time.After(5 * time.Minute)
	done := make(chan error)
	go func() {
		buf := make([]byte, 1)
		n, err := s.Read(buf)
		if err == io.EOF && n == 0 {
			err = nil
		} else if n != 0 {
			err = fmt.Errorf("stream returned data unexpectedly")
		}
		done <- err
		close(done)
	}()
	select {
	case <-n.ServiceContext().Done():
		return n.ServiceContext().Err()
	case <-timeout:
		return fmt.Errorf("timeout")
	case err := <-done:
		return err
	}
}

// WaitForDirectConn registers a notifee with the swarm that
// gets called when we established a direct connection to the
// given peer. Then it waits with a timeout until we actually
// have a direct connection.
func (n *Node) WaitForDirectConn(peerID peer.ID) error {
	// exit early
	if n.HasDirectConnection(peerID) {
		return nil
	}

	// directConnChan receives a signal if a new connection was opened
	// for the given peerID that is a non-relayed address.
	directConnChan := make(chan struct{})
	bundle := &network.NotifyBundle{
		ConnectedF: func(n network.Network, conn network.Conn) {
			if conn.RemotePeer() != peerID {
				return
			}

			if IsRelayAddress(conn.RemoteMultiaddr()) {
				return
			}

			directConnChan <- struct{}{}
		},
	}

	n.Network().Notify(bundle)
	defer func() {
		n.Network().StopNotify(bundle)

		// unstuck any "Connected" notifee functions
		for {
			select {
			case <-directConnChan:
				continue
			default:
			}
			break
		}
		close(directConnChan)
	}()

	// A direct connection could have been established after we
	// checked if we have a direct connection, and before we
	// registered the notifee.
	if n.HasDirectConnection(peerID) {
		return nil
	}

	log.Debugln("Waiting for direct connection...")
	select {
	case <-n.RegisterHolePunchFailed(peerID):
		// hole punching failed :/
		state, found := n.HolePunchStates()[peerID]
		if found {
			return state.Err
		} else {
			return fmt.Errorf("unknown reason")
		}
	case <-directConnChan:
		// we have a direct connection!
		return nil
	case <-time.After(15 * time.Second):
		// we ran into a timeout :/
		return fmt.Errorf("timed out after 15s")
	case <-n.ServiceContext().Done():
		// we were instructed to shut down
		return n.ServiceContext().Err()
	}
}

// DebugLogAuthenticatedPeer prints information about the connection and remote peer to the console.
func (n *Node) DebugLogAuthenticatedPeer(peerID peer.ID) {
	log.Debugln("Authenticated peer:", peerID)

	log.Debugln("Connections:")
	for i, conn := range n.Network().ConnsToPeer(peerID) {
		log.Debugf("[%d] Direction: %s Transient: %s\n", i, conn.Stat().Direction, strconv.FormatBool(conn.Stat().Transient))
		log.Debugln("     Local:  ", conn.LocalMultiaddr())
		log.Debugln("     Remote: ", conn.RemoteMultiaddr())
	}

	protocols, err := n.Network().Peerstore().GetProtocols(peerID)
	if err == nil {
		log.Debugln("Protocols:")
		for _, p := range protocols {
			log.Debugf("  %s\n", p)
		}
	}
	av, err := n.Network().Peerstore().Get(peerID, "AgentVersion")
	if err != nil {
		return
	}

	agentVersion, ok := av.(string)
	if ok {
		log.Debugln("Agent version:", agentVersion)
	}
}

// IsRelayAddress returns true if the given multiaddress contains the /p2p-circuit protocol.
func IsRelayAddress(maddr ma.Multiaddr) bool {
	_, err := maddr.ValueForProtocol(ma.P_CIRCUIT)
	return err == nil
}

// HasDirectConnection returns true if we have at least one non-relay connection to the given peer.
func (n *Node) HasDirectConnection(p peer.ID) bool {
	for _, conn := range n.Network().ConnsToPeer(p) {
		if !IsRelayAddress(conn.RemoteMultiaddr()) {
			return true
		}
	}
	return false
}

// HasHolePunchFailed returns true if the hole punch state is Failed. In case we don't
// track the given peer it returns false.
func (n *Node) HasHolePunchFailed(p peer.ID) bool {
	state, found := n.HolePunchStates()[p]
	if !found {
		return false
	}
	return state.Stage == HolePunchStageFailed
}

// CloseRelayedConnections loops through all known connections to the given peer and
// closes all that are relayed.
func (n *Node) CloseRelayedConnections(p peer.ID) {
	// Close relayed connections
	for _, conn := range n.Network().ConnsToPeer(p) {
		if IsRelayAddress(conn.RemoteMultiaddr()) {
			log.Debugln("Closing relay connection:", conn.RemoteMultiaddr())
			if err := conn.Close(); err != nil {
				log.Warningln("error closing relay connection:", err)
			}
		}
	}
}

package node

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"

	"github.com/multiformats/go-varint"
	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/crypt"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
	"github.com/dennis-tra/pcp/pkg/service"
	"github.com/dennis-tra/pcp/pkg/words"
)

// Is set to true during test runs because the
// generated peers won't have proper keys
var skipMessageAuth = false

type State string

const (
	Idle        State = "idle"
	Discovering       = "discovering"
	Advertising       = "advertising"
	Connected         = "connected"
)

// Node encapsulates the logic for sending and receiving messages.
type Node struct {
	host.Host
	*PushProtocol
	*TransferProtocol
	*PakeProtocol
	*service.Service

	// The public key of this node for easy access
	pubKey []byte

	// DHT is an accessor that is needed in the DHT discoverer/advertiser.
	DHT *kaddht.IpfsDHT

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

	node := &Node{
		Service: service.New("node"),
		state:   Idle,
		stateLk: &sync.RWMutex{},
		Words:   wrds,
		ChanID:  ints[0],
	}
	node.PushProtocol = NewPushProtocol(node)
	node.TransferProtocol = NewTransferProtocol(node)
	node.PakeProtocol, err = NewPakeProtocol(node, wrds)
	if err != nil {
		return nil, err
	}

	key, pub, err := crypto.GenerateKeyPair(crypto.Secp256k1, 256)
	if err != nil {
		return nil, err
	}

	node.pubKey, err = pub.Raw()
	if err != nil {
		return nil, err
	}

	opts = append(opts,
		libp2p.Identity(key),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			node.DHT, err = kaddht.New(c.Context, h)
			return node.DHT, err
		}),

		libp2p.EnableHolePunching(),
	)

	node.Host, err = libp2p.New(opts...)
	if err != nil {
		return nil, err
	}

	return node, node.ServiceStarted()
}

func (n *Node) Shutdown() {
	if err := n.Host.Close(); err != nil {
		log.Warningln("error closing node", err)
	}

	n.ServiceStopped()
}

func (n *Node) SetState(s State) State {
	log.Debugln("Setting local node state to", s)
	n.stateLk.Lock()
	defer n.stateLk.Unlock()
	n.state = s
	return n.state
}

func (n *Node) GetState() State {
	n.stateLk.RLock()
	defer n.stateLk.RUnlock()
	return n.state
}

// Send prepares the message msg to be sent over the network stream s.
// Send closes the stream for writing but leaves it open for reading.
func (n *Node) Send(s network.Stream, msg p2p.HeaderMessage) error {
	defer func() {
		if err := s.CloseWrite(); err != nil {
			log.Warningln("Error closing writer part of stream after sending", err)
		}
	}()

	// Get own public key.
	pub, err := crypto.MarshalPublicKey(n.Host.Peerstore().PubKey(n.Host.ID()))
	if err != nil {
		return err
	}

	hdr := &p2p.Header{
		RequestId:  uuid.New().String(),
		NodeId:     peer.Encode(n.Host.ID()),
		NodePubKey: pub,
		Timestamp:  time.Now().Unix(),
	}
	msg.SetHeader(hdr)
	log.Debugf("Sending message %T to %s with request ID %s\n", msg, s.Conn().RemotePeer().String(), hdr.RequestId)

	// Transform msg to binary to calculate the signature.
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	// Sign the data and attach the signature.
	key := n.Host.Peerstore().PrivKey(n.Host.ID())
	signature, err := key.Sign(data)
	if err != nil {
		return err
	}
	hdr.Signature = signature
	msg.SetHeader(hdr) // Maybe unnecessary

	// Transform msg + signature to binary.
	data, err = proto.Marshal(msg)
	if err != nil {
		return err
	}

	// Encrypt the data with the PAKE session key if it is found
	sKey, found := n.GetSessionKey(s.Conn().RemotePeer())
	if found {
		data, err = crypt.Encrypt(sKey, data)
		if err != nil {
			return err
		}
	}

	// Transmit the data.
	_, err = s.Write(data)
	if err != nil {
		return err
	}

	return nil
}

// authenticateMessage verifies the authenticity of the message payload.
// It takes the given signature and verifies it against the given public
// key.
func (n *Node) authenticateMessage(msg p2p.HeaderMessage) (bool, error) {
	// This will be set to true during unit test runs as the
	// generated peers from mocknet won't have proper keys.
	if skipMessageAuth {
		return true, nil
	}

	// store a temp ref to signature and remove it from message msg
	// sign is a string to allow easy reset to zero-value (empty string)
	signature := msg.GetHeader().Signature
	msg.GetHeader().Signature = nil

	// marshall msg without the signature to protobufs3 binary format
	bin, err := proto.Marshal(msg)
	if err != nil {
		return false, err
	}

	// restore sig in message msg (for possible future use)
	msg.GetHeader().Signature = signature

	// restore peer id binary format from base58 encoded node id msg
	peerID, err := peer.Decode(msg.GetHeader().NodeId)
	if err != nil {
		return false, err
	}

	key, err := crypto.UnmarshalPublicKey(msg.GetHeader().NodePubKey)
	if err != nil {
		return false, fmt.Errorf("failed to extract key from message key msg")
	}

	// extract node id from the provided public key
	idFromKey, err := peer.IDFromPublicKey(key)
	if err != nil {
		return false, fmt.Errorf("failed to extract peer id from public key")
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
	defer s.CloseRead()

	data, err := ioutil.ReadAll(s)
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
		return err
	}
	log.Debugf("type %T with request ID %s\n", buf, buf.GetHeader().RequestId)

	valid, err := n.authenticateMessage(buf)
	if err != nil {
		return err
	}

	if !valid {
		return fmt.Errorf("failed to authenticate message")
	}

	return nil
}

// WriteBytes writes the given bytes to the destination writer and
// prefixes it with a uvarint indicating the length of the data.
func (n *Node) WriteBytes(w io.Writer, data []byte) (int, error) {
	size := varint.ToUvarint(uint64(len(data)))
	return w.Write(append(size, data...))
}

// ReadBytes reads an uvarint from the source reader to know how
// much data is following.
func (n *Node) ReadBytes(r io.Reader) ([]byte, error) {
	br := bufio.NewReader(r) // init byte reader
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
// anymore.
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
// there is a 10 second timeout.
func (n *Node) WaitForEOF(s network.Stream) error {
	log.Debugln("Waiting for stream reset from peer...")
	timeout := time.After(3 * time.Minute)
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
	case <-timeout:
		return fmt.Errorf("timeout")
	case err := <-done:
		return err
	}
}

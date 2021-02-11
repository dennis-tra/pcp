package node

import (
	"bufio"
	"context"
	"fmt"
	"go.uber.org/atomic"
	"io"
	"io/ioutil"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/routing"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/multiformats/go-varint"
	"github.com/pkg/errors"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/config"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
)

// Node encapsulates the logic for sending and receiving messages.
type Node struct {
	host.Host
	*PushProtocol
	*TransferProtocol

	authenticatedPeers sync.Map

	// DHT is an accessor that is needed in the DHT discoverer/advertiser.
	DHT *kaddht.IpfsDHT

	//
	done chan struct{}

	stopped *atomic.Bool
}

// Init creates a new, fully initialized node with the given options.
func Init(ctx context.Context, opts ...libp2p.Option) (*Node, error) {

	conf, err := config.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	if !conf.Identity.IsInitialized() {
		err = conf.Identity.GenerateKeyPair()
		if err != nil {
			return nil, err
		}
	}

	node := &Node{
		authenticatedPeers: sync.Map{},
		done:               make(chan struct{}),
		stopped:            atomic.NewBool(false),
	}
	node.PushProtocol = NewPushProtocol(node)
	node.TransferProtocol = NewTransferProtocol(node)

	key, err := conf.Identity.PrivateKey()
	if err != nil {
		return nil, err
	}

	opts = append(opts,
		libp2p.Identity(key),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			node.DHT, err = kaddht.New(ctx, h)
			return node.DHT, err
		}),
	)

	node.Host, err = libp2p.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	return node, nil
}

func (n *Node) AddAuthenticatedPeer(peerID peer.ID) {
	n.authenticatedPeers.Store(peerID, struct{}{})
}

func (n *Node) IsAuthenticated(peerID peer.ID) bool {
	_, found := n.authenticatedPeers.Load(peerID)
	return found
}

func (n *Node) Ctx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-n.done
		cancel()
	}()
	return ctx
}

func (n *Node) Shutdown() {
	if n.stopped.Load() {
		return
	}
	n.stopped.Store(true)

	if err := n.Host.Close(); err != nil {
		log.Warningln("error closing node", err)
	}

	close(n.done)
}

func (n *Node) Done() chan struct{} {
	// TODO: make it impossible to close this chan from extern
	return n.done
}

// AdvertiseIdentifier returns the string, that we use to advertise
// via mDNS and the DHT. See ChannelID above for more information.
func (n *Node) AdvertiseIdentifier(t time.Time, chanID int16) string {
	return fmt.Sprintf("/pcp/%d/%d", t.Truncate(24*time.Minute).Unix(), chanID)
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
	pub, err := n.Host.Peerstore().PubKey(n.Host.ID()).Bytes()
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
func (n *Node) Read(s network.Stream, data p2p.HeaderMessage) error {

	defer s.CloseRead()
	buf, err := ioutil.ReadAll(s)
	if err != nil {
		if err2 := s.Reset(); err2 != nil {
			err = errors.Wrap(err, err2.Error())
		}
		return err
	}

	if err = proto.Unmarshal(buf, data); err != nil {
		return err
	}

	valid, err := n.authenticateMessage(data)
	if err != nil {
		return err
	}

	if !valid {
		return fmt.Errorf("failed to authenticate message")
	}

	return nil
}

func (n *Node) WriteBytes(w io.Writer, data []byte) (int, error) {

	hdr := varint.ToUvarint(uint64(len(data)))
	nhdr, err := w.Write(hdr)
	if err != nil {
		return nhdr, err
	}

	ndata, err := w.Write(data)
	if err != nil {
		return ndata, err
	}

	return nhdr + ndata, nil
}

func (n *Node) ReadBytes(r io.Reader) ([]byte, error) {
	l, err := varint.ReadUvarint(bufio.NewReader(r))
	if err != nil {
		return nil, err
	}

	buf := make([]byte, l)
	if _, err = r.Read(buf); err != nil {
		return nil, err
	}

	return buf, nil
}

// WaitForEOF waits for an EOF signal on the stream. This indicates that the peer
// has received all data and won't read from this stream anymore. Alternatively
// there is a 10 second timeout.
func (n *Node) WaitForEOF(s network.Stream) error {
	timeout := time.After(10 * time.Second)
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

package node

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"

	"github.com/dennis-tra/pcp/internal/log"
	"github.com/dennis-tra/pcp/pkg/config"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
)

// authenticateMessages is used in tests to skip
// message authentication due to bogus keys.
var authenticateMessages = true

// Node encapsulates the logic for sending and receiving messages.
type Node struct {
	host.Host
	*MdnsProtocol
	*PushProtocol
	*TransferProtocol
}

// Init creates a new, fully initialized node with the given options.
func Init(ctx context.Context, opts ...libp2p.Option) (*Node, error) {

	conf, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}

	if !conf.Identity.IsInitialized() {
		err = conf.Identity.GenerateKeyPair()
		if err != nil {
			return nil, err
		}
	}

	key, err := conf.Identity.PrivateKey()
	if err != nil {
		return nil, err
	}
	opts = append(opts, libp2p.Identity(key))
	h, err := libp2p.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	node := &Node{Host: h}
	node.MdnsProtocol = NewMdnsProtocol(node)
	node.PushProtocol = NewPushProtocol(node)
	node.TransferProtocol = NewTransferProtocol(node)

	return node, nil
}

// ProtocolFor returns the protocol identifier for the given
// form of protobuf message.
func (n *Node) ProtocolFor(msg interface{}) (protocol.ID, error) {
	switch msg.(type) {
	case *p2p.TransferAcknowledge:
		return ProtocolTransferAck, nil
	case *p2p.PushRequest:
		return ProtocolPushRequest, nil
	case *p2p.PushResponse:
		return ProtocolPushResponse, nil
	default:
		return "", fmt.Errorf("unsupported message payload type")
	}
}

func (n *Node) SendProto(ctx context.Context, id peer.ID, msg p2p.HeaderMessage) (string, error) {
	return n.SendProtoWithParentId(ctx, id, msg, "")
}

// SendProto takes the given message and transfers it to the given peer.
// This method signs the message to be authenticated by the peer. It returns
// a unique request ID for this message.
func (n *Node) SendProtoWithParentId(ctx context.Context, id peer.ID, msg p2p.HeaderMessage, parentId string) (string, error) {

	// Get own public key.
	pub, err := n.Host.Peerstore().PubKey(n.Host.ID()).Bytes()
	if err != nil {
		return "", err
	}

	hdr := &p2p.Header{
		RequestParentId: parentId,
		RequestId:       uuid.New().String(),
		NodeId:          peer.Encode(n.Host.ID()),
		NodePubKey:      pub,
		Timestamp:       appTime.Now().Unix(),
	}
	msg.SetHeader(hdr)

	// Determine protocol.
	p, err := n.ProtocolFor(msg)
	if err != nil {
		return "", err
	}

	// Open a logical stream, reusing an existing connection if present.
	s, err := n.NewStream(ctx, id, p)
	if err != nil {
		return "", err
	}

	// Transform msg to binary to calculate the signature.
	data, err := proto.Marshal(msg)
	if err != nil {
		return "", err
	}

	// Sign the data and attach the signature.
	key := n.Host.Peerstore().PrivKey(n.Host.ID())
	signature, err := key.Sign(data)
	if err != nil {
		return "", err
	}
	hdr.Signature = signature
	msg.SetHeader(hdr) // Maybe unnecessary

	// Transform msg + signature to binary.
	data, err = proto.Marshal(msg)
	if err != nil {
		return "", err
	}

	// Transmit the data.
	_, err = s.Write(data)
	if err != nil {
		return "", err
	}

	if err = s.Close(); err != nil {
		log.Infoln("error closing stream", err)
	}

	return msg.GetHeader().RequestId, nil
}

// authenticateMessage verifies the authenticity of the message payload.
// It takes the given signature and verifies it against the given public
// key.
func (n *Node) authenticateMessage(msg p2p.HeaderMessage) (bool, error) {

	// Short circuit in test runs with bogus keys.
	if !authenticateMessages {
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
	peerId, err := peer.Decode(msg.GetHeader().NodeId)
	if err != nil {
		return false, err
	}

	// TODO: The NodePubKey also needs to be checked against the one we're assuming.
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
	if idFromKey != peerId {
		return false, fmt.Errorf("node id and provided public key mismatch")
	}

	return key.Verify(bin, signature)
}

// readMessage drains the given stream and parses the content in the unmarshalls
// it into the protobuf object. It also verifies the authenticity of the message.
func (n *Node) readMessage(s network.Stream, data p2p.HeaderMessage) error {
	buf, err := ioutil.ReadAll(s)
	if err != nil {
		err2 := s.Reset()
		if err2 != nil {
			fmt.Fprintln(os.Stderr, err2)
		}
		return err
	}

	err = s.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error closing stream:", err)
	}

	err = proto.Unmarshal(buf, data)
	if err != nil {
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

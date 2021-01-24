package node

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/dennis-tra/pcp/pkg/config"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
)

// Node represents the construct to send messages to the
// receiving peer. Stream is a bufio.ReadWriter because
// in order to read an uvarint we need an io.ByteReader.
type Node struct {
	host.Host
	*MdnsProtocol
	*SendProtocol
	*TransferProtocol
	stream *bufio.ReadWriter
}

// InitSending creates a new node connected to the given peer.
// Every subsequent call to send will transmit messages to
// this peer.
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
	node.SendProtocol = NewSendProtocol(node)
	node.TransferProtocol = NewTransferProtocol(node)

	return node, nil
}

func (n *Node) SendProto(ctx context.Context, id peer.ID, p protocol.ID, msg *p2p.Header) error {

	s, err := n.NewStream(ctx, id, p)
	if err != nil {
		return err
	}

	// Transform msg to binary to calculate the signature.
	data, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	key := n.Host.Peerstore().PrivKey(n.Host.ID())
	res, err := key.Sign(data)
	if err != nil {
		return err
	}
	msg.Signature = res

	// Transform msg + signature to binary.
	data, err = proto.Marshal(msg)
	if err != nil {
		return err
	}

	// Then transmit the data.
	_, err = s.Write(data)
	if err != nil {
		return err
	}

	return s.Close()
}

func (n *Node) NewHeader() (*p2p.Header, error) {
	pub, err := n.Host.Peerstore().PubKey(n.Host.ID()).Bytes()
	if err != nil {
		return nil, err
	}

	return &p2p.Header{
		RequestId:  uuid.New().String(),
		NodeId:     peer.Encode(n.Host.ID()),
		NodePubKey: pub,
		Timestamp:  time.Now().Unix(),
	}, nil
}

func (n *Node) NewSendResponse(accept bool) (*p2p.SendResponse, error) {
	return &p2p.SendResponse{Accept: accept}, nil
}

func (n *Node) NewSendRequest(fileName string, fileSize int64, c cid.Cid) (*p2p.SendRequest, error) {
	return &p2p.SendRequest{
		FileName: fileName,
		FileSize: fileSize,
		Cid:      c.Bytes(),
	}, nil
}

func (n *Node) authenticateMessage(data *p2p.Header) (bool, error) {
	// store a temp ref to signature and remove it from message data
	// sign is a string to allow easy reset to zero-value (empty string)
	signature := data.Signature
	data.Signature = nil

	// marshall data without the signature to protobufs3 binary format
	bin, err := proto.Marshal(data)
	if err != nil {
		return false, err
	}

	// restore sig in message data (for possible future use)
	data.Signature = signature

	// restore peer id binary format from base58 encoded node id data
	peerId, err := peer.Decode(data.NodeId)
	if err != nil {
		return false, err
	}

	key, err := crypto.UnmarshalPublicKey(data.NodePubKey)
	if err != nil {
		return false, fmt.Errorf("failed to extract key from message key data")
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

func (n *Node) readMessage(s network.Stream) (*p2p.Header, error) {
	buf, err := ioutil.ReadAll(s)
	if err != nil {
		err2 := s.Reset()
		if err2 != nil {
			fmt.Println(err2)
		}
		return nil, err
	}
	err = s.Close()
	if err != nil {
		fmt.Println("Error closing stream:", err)
	}

	data := &p2p.Header{}
	err = proto.Unmarshal(buf, data)
	if err != nil {
		return nil, err
	}

	valid, err := n.authenticateMessage(data)
	if err != nil {
		return nil, err
	}

	if !valid {
		return nil, fmt.Errorf("failed to authenticate message")
	}

	return data, nil
}

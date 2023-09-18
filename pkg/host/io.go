package host

import (
	"fmt"
	"strconv"
	"time"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-varint"
	"github.com/sirupsen/logrus"

	"github.com/dennis-tra/pcp/pkg/crypt"
	"github.com/dennis-tra/pcp/pkg/io"
	p2p "github.com/dennis-tra/pcp/pkg/pb"
)

// Send prepares the message msg to be sent over the network stream s.
// Send closes the stream for writing but leaves it open for reading.
func (m *Model) Send(s network.Stream, msg p2p.HeaderMessage) error {
	// Get own public key.
	pub, err := crypto.MarshalPublicKey(m.Host.Peerstore().PubKey(m.Host.ID()))
	if err != nil {
		return fmt.Errorf("marshal public key: %w", err)
	}

	hdr := &p2p.Header{
		RequestId:  uuid.New().String(),
		NodeId:     m.Host.ID().String(),
		NodePubKey: pub,
		Timestamp:  time.Now().Unix(),
	}
	msg.SetHeader(hdr)

	log.WithFields(logrus.Fields{
		"requestID": hdr.RequestId,
		"remoteID":  s.Conn().RemotePeer().String(),
	}).Debugf("Sending message %T\n", msg)

	// Transform msg to binary to calculate the signature.
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal proto message %T: %w", msg, err)
	}

	// Sign the data and attach the signature.
	key := m.Host.Peerstore().PrivKey(m.Host.ID())
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
	sKey := m.AuthProt.GetSessionKey(s.Conn().RemotePeer())
	if len(sKey) == 0 {
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
func (m *Model) authenticateMessage(msg p2p.HeaderMessage) (bool, error) {
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

	// restore peer id binary format from base58 encoded host id msg
	peerID, err := peer.Decode(msg.GetHeader().NodeId)
	if err != nil {
		return false, fmt.Errorf("decode msg peer id: %w", err)
	}

	key, err := crypto.UnmarshalPublicKey(msg.GetHeader().NodePubKey)
	if err != nil {
		return false, fmt.Errorf("extract key from message key msg: %w", err)
	}

	// extract host id from the provided public key
	idFromKey, err := peer.IDFromPublicKey(key)
	if err != nil {
		return false, fmt.Errorf("extract peer id from public key: %w", err)
	}

	// verify that message author host id matches the provided host public key
	if idFromKey != peerID {
		return false, fmt.Errorf("host id and provided public key mismatch")
	}

	return key.Verify(bin, signature)
}

// Read drains the given stream and parses the content. It unmarshalls
// it into the protobuf object. It also verifies the authenticity of the message.
// Read closes the stream for reading but leaves it open for writing.
func (m *Model) Read(s network.Stream, buf p2p.HeaderMessage) error {
	data, err := io.ReadBytes(s)
	if err != nil {
		if err2 := s.Reset(); err2 != nil {
			err = fmt.Errorf("%s: %w", err2.Error(), err)
		}
		return err
	}

	log.Debugf("Reading message from %s\n", s.Conn().RemotePeer().String())
	// Decrypt the data with the PAKE session key if it is found
	sKey := m.AuthProt.GetSessionKey(s.Conn().RemotePeer())
	if len(sKey) == 0 {
		data, err = crypt.Decrypt(sKey, data)
		if err != nil {
			return err
		}
	}

	if err = proto.Unmarshal(data, buf); err != nil {
		return fmt.Errorf("unmarshal received data: %w", err)
	}
	log.Debugf("type %T with request ID %s\n", buf, buf.GetHeader().RequestId)

	valid, err := m.authenticateMessage(buf)
	if err != nil {
		return fmt.Errorf("authenticate msg: %w", err)
	} else if !valid {
		return fmt.Errorf("failed to authenticate message")
	}

	return nil
}

// debugLogAuthenticatedPeer prints information about the connection and remote peer to the console.
func (m *Model) debugLogAuthenticatedPeer(peerID peer.ID) {
	log.Debugln("Authenticated peer:", peerID)

	log.Debugln("Connections:")
	for i, conn := range m.Network().ConnsToPeer(peerID) {
		log.Debugf("[%d] Direction: %s Transient: %s\n", i, conn.Stat().Direction, strconv.FormatBool(conn.Stat().Transient))
		log.Debugln("     Local:  ", conn.LocalMultiaddr())
		log.Debugln("     Remote: ", conn.RemoteMultiaddr())
	}

	protocols, err := m.Network().Peerstore().GetProtocols(peerID)
	if err == nil {
		log.Debugln("Protocols:")
		for _, p := range protocols {
			log.Debugf("  %s\n", p)
		}
	}
	av, err := m.Network().Peerstore().Get(peerID, "AgentVersion")
	if err != nil {
		return
	}

	agentVersion, ok := av.(string)
	if ok {
		log.Debugln("Agent version:", agentVersion)
	}
}

// isRelayAddress returns true if the given multiaddress contains the /p2p-circuit protocol.
func isRelayAddress(maddr ma.Multiaddr) bool {
	_, err := maddr.ValueForProtocol(ma.P_CIRCUIT)
	return err == nil
}

package host

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/dennis-tra/pcp/internal/mock"
	"github.com/dennis-tra/pcp/pkg/discovery"
)

type voidSender struct {
	msgChan chan tea.Msg
}

func (v voidSender) Send(tea.Msg) {
}

var _ tea.Sender = (*voidSender)(nil)

type AuthProtocolTestSuite struct {
	suite.Suite

	ctx  context.Context
	ctrl *gomock.Controller

	senderChan   voidSender
	receiverChan voidSender

	sender   *AuthProtocol
	receiver *AuthProtocol

	senderHost   *mock.MockHost
	receiverHost *mock.MockHost

	receiverPublicKey crypto.PubKey
	senderPublicKey   crypto.PubKey
}

func (suite *AuthProtocolTestSuite) SetupSuite() {
	logrus.SetLevel(logrus.PanicLevel)
}

func (suite *AuthProtocolTestSuite) TearDownSuite() {
	logrus.SetLevel(logrus.InfoLevel)
}

func (suite *AuthProtocolTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.ctrl = gomock.NewController(suite.T())

	receiverPeerstore, err := pstoremem.NewPeerstore()
	suite.Require().NoError(err)
	senderPeerstore, err := pstoremem.NewPeerstore()
	suite.Require().NoError(err)

	senderHost := mock.NewMockHost(suite.ctrl)
	senderHost.EXPECT().ID().Return(suite.newPeerID()).AnyTimes()
	senderHost.EXPECT().SetStreamHandler(gomock.Eq(ProtocolAuthRoleReceiver), gomock.Any()).AnyTimes()
	senderHost.EXPECT().Close().AnyTimes()
	senderHost.EXPECT().RemoveStreamHandler(gomock.Eq(ProtocolAuthRoleReceiver)).AnyTimes()
	senderHost.EXPECT().Peerstore().Return(senderPeerstore).AnyTimes()

	receiverHost := mock.NewMockHost(suite.ctrl)
	receiverHost.EXPECT().ID().Return(suite.newPeerID()).AnyTimes()
	receiverHost.EXPECT().SetStreamHandler(gomock.Eq(ProtocolAuthRoleSender), gomock.Any()).AnyTimes()
	receiverHost.EXPECT().Close().AnyTimes()
	receiverHost.EXPECT().RemoveStreamHandler(gomock.Eq(ProtocolAuthRoleSender)).AnyTimes()
	receiverHost.EXPECT().Peerstore().Return(receiverPeerstore).AnyTimes()

	words := []string{"silly", "silly", "silly"}

	suite.senderHost = senderHost
	suite.receiverHost = receiverHost

	suite.senderChan = voidSender{msgChan: make(chan tea.Msg)}
	suite.receiverChan = voidSender{msgChan: make(chan tea.Msg)}

	suite.sender = NewAuthProtocol(suite.ctx, senderHost, suite.senderChan, discovery.RoleSender, words)
	suite.receiver = NewAuthProtocol(suite.ctx, receiverHost, suite.receiverChan, discovery.RoleReceiver, words)

	suite.sender.RegisterKeyExchangeHandler()
	suite.receiver.RegisterKeyExchangeHandler()
}

func (suite *AuthProtocolTestSuite) newPeerID() peer.ID {
	sk, _, err := crypto.GenerateEd25519Key(rand.Reader)
	suite.Require().NoError(err)

	id, err := peer.IDFromPrivateKey(sk)
	suite.Require().NoError(err)

	return id
}

func (suite *AuthProtocolTestSuite) TearDownTest() {
	suite.sender.UnregisterKeyExchangeHandler()
	suite.receiver.UnregisterKeyExchangeHandler()

	err := suite.sender.host.Close()
	suite.NoError(err)

	err = suite.receiver.host.Close()
	suite.NoError(err)
}

func (suite *AuthProtocolTestSuite) TestAuthMsg_StartKeyExchange_updateAuthStep() {
	// The receiving peer starts the key exchange
	// -> discard command which would actually open the stream
	//    and do the key exchange
	_ = suite.receiver.StartKeyExchange(suite.ctx, suite.sender.host.ID())

	// check if the state was properly initialized
	state := suite.receiver.states[suite.sender.host.ID()]
	suite.Equal(AuthStepStart, state.Step)
	suite.Nil(state.stream)
	suite.Nil(state.Err)
	suite.Nil(state.Key)
}

func (suite *AuthProtocolTestSuite) TestAuthMsg_StartKeyExchange_SenderStarts() {
	_ = suite.sender.StartKeyExchange(suite.ctx, suite.receiver.host.ID())

	recToSenConn := mock.NewMockConn(suite.ctrl)
	recToSenConn.EXPECT().RemotePeer().Return(suite.sender.host.ID())

	recInStream := mock.NewMockStream(suite.ctrl)
	recInStream.EXPECT().Conn().Return(recToSenConn)
	recInStream.EXPECT().Reset().Times(0)
	recInStream.EXPECT().ID().Return("1").AnyTimes()

	suite.receiver.Update(authOnKeyExchange{stream: recInStream})

	receiverState := suite.receiver.states[suite.sender.host.ID()]
	suite.Equal(AuthStepStart, receiverState.Step)
	suite.Equal(recInStream.ID(), receiverState.stream.ID())
	suite.Nil(receiverState.Err)
	suite.Len(receiverState.Key, 0)
}

func (suite *AuthProtocolTestSuite) TestAuthMsg_StartKeyExchange_Simultaneous() {
	_ = suite.sender.StartKeyExchange(suite.ctx, suite.receiver.host.ID())
	_ = suite.receiver.StartKeyExchange(suite.ctx, suite.sender.host.ID())

	senToRecConn := mock.NewMockConn(suite.ctrl)
	recToSenConn := mock.NewMockConn(suite.ctrl)

	senToRecConn.EXPECT().RemotePeer().Return(suite.receiver.host.ID())
	recToSenConn.EXPECT().RemotePeer().Return(suite.sender.host.ID())

	senInStream := mock.NewMockStream(suite.ctrl)
	recInStream := mock.NewMockStream(suite.ctrl)

	senInStream.EXPECT().Conn().Return(senToRecConn)
	recInStream.EXPECT().Conn().Return(recToSenConn)

	senInStream.EXPECT().Reset().Times(0)
	recInStream.EXPECT().Reset().Return(nil).Times(1)

	senInStream.EXPECT().ID().Return("1").AnyTimes()
	recInStream.EXPECT().ID().Return("2").AnyTimes()

	suite.sender.Update(authOnKeyExchange{stream: senInStream})
	suite.receiver.Update(authOnKeyExchange{stream: recInStream})

	senderState := suite.sender.states[suite.receiver.host.ID()]
	suite.Equal(AuthStepStart, senderState.Step)
	suite.Equal(senInStream.ID(), senderState.stream.ID())
	suite.Nil(senderState.Err)
	suite.Len(senderState.Key, 0)

	receiverState := suite.receiver.states[suite.sender.host.ID()]
	suite.Equal(AuthStepStart, receiverState.Step)
	suite.Nil(receiverState.stream)
	suite.Nil(receiverState.Err)
	suite.Len(receiverState.Key, 0)
}

func (suite *AuthProtocolTestSuite) TestAuthMsg_StartKeyExchange_NoOpIfInProgress() {
	cmd := suite.sender.StartKeyExchange(suite.ctx, suite.receiver.host.ID())
	suite.NotNil(cmd)

	// Should not start key exchange again if it's still in progress
	cmd = suite.sender.StartKeyExchange(suite.ctx, suite.receiver.host.ID())
	suite.Nil(cmd)

	// Should start key exchange again if we encountered an error
	suite.sender.states[suite.receiver.host.ID()].Step = AuthStepError
	suite.sender.states[suite.receiver.host.ID()].Err = fmt.Errorf("some error")
	cmd = suite.sender.StartKeyExchange(suite.ctx, suite.receiver.host.ID())
	suite.NotNil(cmd)
}

func (suite *AuthProtocolTestSuite) TestAuthMsg_StartKeyExchange_DelayedReceiverStreamOpen() {
	_ = suite.sender.StartKeyExchange(suite.ctx, suite.receiver.host.ID())

	recToSenConn := mock.NewMockConn(suite.ctrl)
	recToSenConn.EXPECT().RemotePeer().Return(suite.receiver.host.ID())

	senOutStream := mock.NewMockStream(suite.ctrl)
	senOutStream.EXPECT().Reset().Times(1)
	senOutStream.EXPECT().ID().Return("1").AnyTimes()

	suite.sender.Update(authMsg[AuthStep]{
		peerID:  suite.receiver.host.ID(),
		payload: AuthStepExchangingSalt,
		stream:  senOutStream,
	})

	senInStream := mock.NewMockStream(suite.ctrl)
	senInStream.EXPECT().Conn().Return(recToSenConn)
	senInStream.EXPECT().Reset().Times(0)
	senInStream.EXPECT().ID().Return("2").AnyTimes()

	suite.sender.Update(authOnKeyExchange{stream: senInStream})
	senderState := suite.sender.states[suite.receiver.host.ID()]
	suite.Equal(AuthStepStart, senderState.Step)
	suite.Equal(senInStream.ID(), senderState.stream.ID())
	suite.Nil(senderState.Err)
	suite.Len(senderState.Key, 0)
}

func (suite *AuthProtocolTestSuite) TestAuthMsg_StartKeyExchange_FromPeerWhereAuthenticationFailed() {
	suite.sender.states[suite.receiver.host.ID()] = &AuthState{
		Step:   AuthStepError,
		Key:    nil,
		Err:    ErrAuthenticationFailed,
		stream: nil,
	}

	recToSenConn := mock.NewMockConn(suite.ctrl)
	recToSenConn.EXPECT().RemotePeer().Return(suite.receiver.host.ID())

	senInStream := mock.NewMockStream(suite.ctrl)
	senInStream.EXPECT().Conn().Return(recToSenConn)
	senInStream.EXPECT().Reset().Times(1)
	senInStream.EXPECT().ID().Return("1").AnyTimes()

	suite.sender.Update(authOnKeyExchange{stream: senInStream})

	senderState := suite.sender.states[suite.receiver.host.ID()]
	suite.Equal(AuthStepError, senderState.Step)
	suite.Equal(ErrAuthenticationFailed, senderState.Err)
	suite.Len(senderState.Key, 0)
}

func (suite *AuthProtocolTestSuite) TestAuthMsg_error() {
	err := fmt.Errorf("some error")

	msg := authMsg[error]{
		peerID:  suite.receiver.host.ID(),
		payload: err,
		stream:  nil,
	}
	suite.sender.Update(msg)

	senderState := suite.sender.states[suite.receiver.host.ID()]
	suite.Equal(AuthStepError, senderState.Step)
	suite.Equal(err, senderState.Err)
	suite.Nil(senderState.stream)
	suite.Nil(senderState.Key)
}

func (suite *AuthProtocolTestSuite) TestAuthMsg_ErrorForObsoleteStream() {
	_ = suite.sender.StartKeyExchange(suite.ctx, suite.receiver.host.ID())
	_ = suite.receiver.StartKeyExchange(suite.ctx, suite.sender.host.ID())

	senToRecConn := mock.NewMockConn(suite.ctrl)
	recToSenConn := mock.NewMockConn(suite.ctrl)

	senToRecConn.EXPECT().RemotePeer().Return(suite.receiver.host.ID())
	recToSenConn.EXPECT().RemotePeer().Return(suite.sender.host.ID())

	senInStream := mock.NewMockStream(suite.ctrl)
	recInStream := mock.NewMockStream(suite.ctrl)

	senInStream.EXPECT().Conn().Return(senToRecConn)
	recInStream.EXPECT().Conn().Return(recToSenConn)

	senInStream.EXPECT().Reset().Times(0)
	recInStream.EXPECT().Reset().Return(nil).Times(1)

	senInStream.EXPECT().ID().Return("1").AnyTimes()
	recInStream.EXPECT().ID().Return("2").AnyTimes()

	suite.sender.Update(authOnKeyExchange{stream: senInStream})
	suite.receiver.Update(authOnKeyExchange{stream: recInStream})

	suite.sender.Update(authMsg[error]{
		peerID:  suite.receiver.host.ID(),
		payload: fmt.Errorf("stream reset"),
		stream:  recInStream,
	})

	senderState := suite.sender.states[suite.receiver.host.ID()]
	suite.Equal(AuthStepStart, senderState.Step)
	suite.Equal(senInStream.ID(), senderState.stream.ID())
	suite.Nil(senderState.Err)
	suite.Len(senderState.Key, 0)
}

func (suite *AuthProtocolTestSuite) TestAuthMsg_bytes() {
	bts := []byte("some bytes")
	peerID := suite.receiver.host.ID()

	msg := authMsg[[]byte]{
		peerID:  peerID,
		payload: bts,
		stream:  nil,
	}
	suite.sender.Update(msg)

	state := suite.sender.states[peerID]
	suite.Equal(state.Step, AuthStepPeerAuthenticated)
	suite.Nil(state.Err)
	suite.Nil(state.stream)
	suite.Equal(state.Key, bts)
}

func (suite *AuthProtocolTestSuite) TestFullKeyExchange() {
	recReader, recWriter := io.Pipe()
	senReader, senWriter := io.Pipe()

	senToRecConn := mock.NewMockConn(suite.ctrl)
	recToSenConn := mock.NewMockConn(suite.ctrl)

	senToRecConn.EXPECT().RemotePeer().Return(suite.receiver.host.ID()).AnyTimes()
	recToSenConn.EXPECT().RemotePeer().Return(suite.sender.host.ID()).AnyTimes()

	recStream := mock.NewMockStream(suite.ctrl)
	senStream := mock.NewMockStream(suite.ctrl)

	recStream.EXPECT().Conn().Return(recToSenConn).AnyTimes()
	senStream.EXPECT().Conn().Return(senToRecConn).AnyTimes()

	recStream.EXPECT().Close().DoAndReturn(func() error {
		if err := senReader.Close(); err != nil {
			return err
		}
		return recWriter.Close()
	}).AnyTimes()
	senStream.EXPECT().Close().DoAndReturn(func() error {
		if err := recReader.Close(); err != nil {
			return err
		}
		return senWriter.Close()
	}).AnyTimes()

	recStream.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
		return recReader.Read(p)
	}).AnyTimes()
	recStream.EXPECT().Write(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
		return senWriter.Write(p)
	}).AnyTimes()
	senStream.EXPECT().Read(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
		return senReader.Read(p)
	}).AnyTimes()
	senStream.EXPECT().Write(gomock.Any()).DoAndReturn(func(p []byte) (int, error) {
		return recWriter.Write(p)
	}).AnyTimes()

	suite.receiverHost.EXPECT().
		NewStream(gomock.Any(), gomock.Eq(suite.sender.host.ID()), gomock.Eq(ProtocolAuthRoleReceiver)).
		DoAndReturn(func(ctx context.Context, p peer.ID, pids ...protocol.ID) (network.Stream, error) {
			return recStream, nil
		})

	senderPublicKey, err := suite.senderHost.ID().ExtractPublicKey()
	suite.Require().NoError(err)
	err = suite.receiverHost.Peerstore().AddPubKey(suite.senderHost.ID(), senderPublicKey)
	suite.Require().NoError(err)

	receiverPublicKey, err := suite.receiverHost.ID().ExtractPublicKey()
	suite.Require().NoError(err)
	err = suite.senderHost.Peerstore().AddPubKey(suite.receiverHost.ID(), receiverPublicKey)
	suite.Require().NoError(err)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		msg := suite.sender.exchangeKeys(senStream)()
		suite.sender.Update(msg)
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		msg := suite.receiver.StartKeyExchange(context.TODO(), suite.sender.host.ID())()
		suite.receiver.Update(msg)
		wg.Done()
	}()
	wg.Wait()

	senderState := suite.sender.states[suite.receiverHost.ID()]
	suite.Equal(AuthStepPeerAuthenticated, senderState.Step)
	suite.NotNil(senderState.Key)
	suite.Nil(senderState.Err)

	receiverState := suite.receiver.states[suite.senderHost.ID()]
	suite.Equal(AuthStepPeerAuthenticated, receiverState.Step)
	suite.NotNil(receiverState.Key)
	suite.Nil(receiverState.Err)

	suite.Equal(senderState.Key, receiverState.Key)
}

func (suite *AuthProtocolTestSuite) TestAuthMsg_step() {
	peerID := suite.receiver.host.ID()

	stream1 := mock.NewMockStream(suite.ctrl)
	stream1.EXPECT().ID().Return("1").AnyTimes()

	msg := authMsg[AuthStep]{
		peerID:  peerID,
		payload: AuthStepStart,
		stream:  stream1,
	}
	suite.sender.Update(msg)

	state := suite.sender.states[peerID]
	suite.Equal(state.Step, AuthStepStart)
	suite.Equal(stream1, state.stream)
	suite.Nil(state.Err)
	suite.Nil(state.Key)

	msg = authMsg[AuthStep]{
		peerID:  peerID,
		payload: AuthStepExchangingSalt,
		stream:  stream1,
	}
	suite.sender.Update(msg)

	state = suite.sender.states[peerID]
	suite.Equal(state.Step, AuthStepExchangingSalt)
	suite.Equal(stream1, state.stream)
	suite.Nil(state.Err)
	suite.Nil(state.Key)

	stream2 := mock.NewMockStream(suite.ctrl)
	stream2.EXPECT().ID().Return("2").AnyTimes()
	stream2.EXPECT().Reset().Times(1)

	msg = authMsg[AuthStep]{
		peerID:  peerID,
		payload: AuthStepVerifyingProofFromPeer,
		stream:  stream2,
	}
	suite.sender.Update(msg)
	suite.Equal(state.Step, AuthStepExchangingSalt)
	suite.Equal(stream1, state.stream) // stream1 !
	suite.Nil(state.Err)
	suite.Nil(state.Key)

	// set peer to be authenticated
	suite.sender.states[peerID].Step = AuthStepPeerAuthenticated
	suite.sender.states[peerID].Key = []byte("random bytes")

	msg = authMsg[AuthStep]{
		peerID:  peerID,
		payload: AuthStepExchangingSalt,
		stream:  stream1,
	}
	suite.sender.Update(msg)

	state = suite.sender.states[peerID]
	suite.Equal(state.Step, AuthStepPeerAuthenticated)
	suite.Equal(stream1, state.stream)
	suite.Nil(state.Err)
	suite.Equal([]byte("random bytes"), state.Key)
	suite.True(suite.sender.IsAuthenticated(peerID))
	suite.Equal([]byte("random bytes"), suite.sender.GetSessionKey(peerID))

	// set peer to have an "authentication failed" error
	suite.sender.states[peerID].Step = AuthStepError
	suite.sender.states[peerID].Err = ErrAuthenticationFailed
	suite.sender.states[peerID].Key = nil

	msg = authMsg[AuthStep]{
		peerID:  peerID,
		payload: AuthStepExchangingSalt,
		stream:  stream1,
	}
	suite.sender.Update(msg)

	state = suite.sender.states[peerID]
	suite.Equal(state.Step, AuthStepError)
	suite.Equal(stream1, state.stream)
	suite.Equal(ErrAuthenticationFailed, state.Err)
	suite.Nil(state.Key)

	// set peer to have a random error
	suite.sender.states[peerID].Step = AuthStepError
	suite.sender.states[peerID].Err = fmt.Errorf("random error")
	suite.sender.states[peerID].Key = nil

	msg = authMsg[AuthStep]{
		peerID:  peerID,
		payload: AuthStepStart,
		stream:  stream1,
	}
	suite.sender.Update(msg)

	state = suite.sender.states[peerID]
	suite.Equal(state.Step, AuthStepStart)
	suite.Equal(stream1, state.stream)
	suite.Nil(state.Err)
	suite.Nil(state.Key)
}

func (suite *AuthProtocolTestSuite) TestAuthProtocol_PakeStateStr() {
	peerID := suite.receiver.host.ID()

	untrackedStr := suite.sender.PakeStateStr(peerID)
	suite.NotEmpty(untrackedStr)

	suite.sender.states[peerID] = &AuthState{
		Step: AuthStep(99), // non-existent step
	}
	unknownStr := suite.sender.PakeStateStr(peerID)

	steps := []AuthStep{
		AuthStepUnknown,
		AuthStepStart,
		AuthStepWaitingForKeyInformation,
		AuthStepCalculatingKeyInformation,
		AuthStepSendingKeyInformation,
		AuthStepWaitingForFinalKeyInformation,
		AuthStepExchangingSalt,
		AuthStepProvingAuthenticityToPeer,
		AuthStepVerifyingProofFromPeer,
		AuthStepWaitingForFinalConfirmation,
		AuthStepPeerAuthenticated,
		AuthStepError,
	}
	for _, step := range steps {
		suite.T().Run(fmt.Sprintf("test %s", step), func(t *testing.T) {
			suite.sender.states[peerID] = &AuthState{
				Step: step,
				Err:  fmt.Errorf("some error"),
			}
			str := suite.sender.PakeStateStr(peerID)
			suite.NotEmpty(str)
			suite.NotEqual(untrackedStr, str)

			switch step {
			case AuthStepUnknown:
			case AuthStepError:
				suite.Contains(str, "some error")
			default:
				suite.NotEqual(unknownStr, str)
			}
		})
	}
}

func (suite *AuthProtocolTestSuite) TestAuthProtocol_GetSessionKey() {
	peerID := suite.receiver.host.ID()
	key := suite.sender.GetSessionKey(peerID)
	suite.Nil(key)

	suite.sender.states[peerID] = &AuthState{
		Step: AuthStepPeerAuthenticated,
		Key:  []byte("random bytes"),
	}
	key = suite.sender.GetSessionKey(peerID)
	suite.Equal([]byte("random bytes"), key)
}

func TestAuthProtocolTestSuite(t *testing.T) {
	suite.Run(t, new(AuthProtocolTestSuite))
}

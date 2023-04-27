package node

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dennis-tra/pcp/pkg/crypt"
	"github.com/dennis-tra/pcp/pkg/service"
)

// TestTransferHandler is a mock transfer handler that can be registered for the TransferProtocol.
type TestTransferHandler struct {
	handler func(*tar.Header, io.Reader)
	done    func()
}

func (tth *TestTransferHandler) HandleFile(hdr *tar.Header, r io.Reader) { tth.handler(hdr, r) }

func (tth *TestTransferHandler) Done() {
	tth.done()
}

func TestTransferProtocol_onTransfer(t *testing.T) {
	// The test sets up two nodes to simulate a file transfer. In every run node1 send a particular file or whole
	// directory from the top-level tests/ directory to node2. node2 then writes the received data to the tests/tmp/
	// directory. After the file transfer has finished the assertTmpIntegrity checks if the contents of the tests/tmp/
	// directory matches the contents of the particular test directory.
	// The abbreviation obj stands for object and captures the missing distinction between files and directories.
	tests := []struct {
		testObj string // Path as given by user.
		isDir   bool
	}{
		{testObj: "transfer_file/file", isDir: false},
		{testObj: "transfer_dir_empty", isDir: true},
		{testObj: "transfer_dir", isDir: true},
		{testObj: "transfer_subdir", isDir: true},
		{testObj: "transfer_file_subdir/subdir/file", isDir: false},
		//{testObj: "transfer_link", isDir: true}, // doesn't work yet
	}
	for _, tt := range tests {
		t.Run("transfer-test: "+tt.testObj, func(t *testing.T) {
			ctx := context.Background()
			net := mocknet.New()

			node1, _ := setupNode(t, net)
			node2, done := setupNode(t, net)
			authNodes(t, node1, node2)

			err := net.LinkAll()
			require.NoError(t, err)

			err = node1.Transfer(ctx, node2.ID(), relTestDir(tt.testObj))
			require.NoError(t, err)

			assertTmpIntegrity(t, tt.testObj, tt.isDir)

			<-done

			node1.UnregisterTransferHandler()
			node2.UnregisterTransferHandler()
		})
	}
	// Clean up after ourselves
	require.NoError(t, os.RemoveAll(tmpDir()))
}

func TestTransferProtocol_onTransfer_senderNotAuthenticatedAtReceiver(t *testing.T) {
	ctx := context.Background()
	net := mocknet.New()

	node1, _ := setupNode(t, net)
	node2, _ := setupNode(t, net)

	// Simulate that the receiver is authenticated (from the perspective of the sender)
	key, err := crypt.DeriveKey([]byte{}, []byte{})
	require.NoError(t, err)
	node1.PakeProtocol.states[node2.ID()] = &PakeState{Step: PakeStepStart, Key: key}

	err = net.LinkAll()
	require.NoError(t, err)

	err = node1.Transfer(ctx, node2.ID(), relTestDir("transfer_file/file"))
	require.Error(t, err)
}

func TestTransferProtocol_onTransfer_peersDifferentKeys(t *testing.T) {
	ctx := context.Background()
	net := mocknet.New()

	node1, _ := setupNode(t, net)
	node2, _ := setupNode(t, net)

	// Simulate that the receiver is authenticated (from the perspective of the sender)
	key1, err := crypt.DeriveKey([]byte{1}, []byte{1})
	require.NoError(t, err)
	key2, err := crypt.DeriveKey([]byte{2}, []byte{2})
	require.NoError(t, err)
	node1.PakeProtocol.states[node2.ID()] = &PakeState{Step: PakeStepStart, Key: key1}
	node1.PakeProtocol.states[node1.ID()] = &PakeState{Step: PakeStepStart, Key: key2}

	err = net.LinkAll()
	require.NoError(t, err)

	err = node1.Transfer(ctx, node2.ID(), relTestDir("transfer_file/file"))
	fmt.Println(err)
	require.Error(t, err)
}

func TestTransferProtocol_onTransfer_provokeErrCases(t *testing.T) {
	ctx := context.Background()
	net := mocknet.New()

	node1, _ := setupNode(t, net)
	node2, _ := setupNode(t, net)

	node1.RegisterTransferHandler(&TestTransferHandler{handler: tmpWriter(t), done: func() {}})
	node2.RegisterTransferHandler(&TestTransferHandler{handler: tmpWriter(t), done: func() {}})

	// Can't create stream
	err := node1.Transfer(ctx, "some-non-existing-node", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot connect")

	err = net.LinkAll()
	require.NoError(t, err)

	// Can't read object that the user wants to send
	err = node1.Transfer(ctx, node2.ID(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no such file or directory")

	// Receiving peer is unauthenticated
	err = node1.Transfer(ctx, node2.ID(), relTestDir("transfer_file/file"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session key not found")

	// Session key has wrong format
	node1.PakeProtocol.states[node2.ID()] = &PakeState{Step: PakeStepStart, Key: []byte{1, 2, 3}}
	err = node1.Transfer(ctx, node2.ID(), relTestDir("transfer_file/file"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key size 3")
}

// setupNode builds a node ready to handle a transfer.
func setupNode(t *testing.T, net mocknet.Mocknet) (*Node, chan struct{}) {
	p, err := net.GenPeer()
	require.NoError(t, err)
	n := &Node{Service: service.New("node"), Host: p}
	n.PakeProtocol, err = NewPakeProtocol(n, []string{"equal", "equal", "equal", "equal"})
	require.NoError(t, err)
	n.TransferProtocol = NewTransferProtocol(n)
	done := make(chan struct{})
	n.RegisterTransferHandler(&TestTransferHandler{handler: tmpWriter(t), done: func() { close(done) }})
	n.PushProtocol = NewPushProtocol(n)
	return n, done
}

// authNodes generates a key and uses it to simulate that the two given nodes have a shared key aka are authenticated.
func authNodes(t *testing.T, node1 *Node, node2 *Node) {
	key, err := crypt.DeriveKey([]byte{}, []byte{})
	require.NoError(t, err)
	node1.PakeProtocol.addSessionKey(node2.ID(), key)
	node2.PakeProtocol.addSessionKey(node1.ID(), key)
}

func Test_relPath(t *testing.T) {
	tests := []struct {
		basePath   string // Path given by user.
		baseIsDir  bool
		targetPath string // Path retrieved from "walking" basePath.
		wantPath   string // Path we want to put in the tar - this is how the files are extracted on the receiving side.
	}{
		{basePath: "file", baseIsDir: false, targetPath: "file", wantPath: "file"},
		{basePath: "a/file", baseIsDir: false, targetPath: "a/file", wantPath: "file"},
		{basePath: "../../file", baseIsDir: false, targetPath: "../../file", wantPath: "file"},
		{basePath: "../a", baseIsDir: true, targetPath: "../a/file", wantPath: "a/file"},
		{basePath: "a/", baseIsDir: true, targetPath: "a/file", wantPath: "a/file"},
		{basePath: "a", baseIsDir: true, targetPath: "a", wantPath: "a"},
		{basePath: "a/b/", baseIsDir: true, targetPath: "a/b/file", wantPath: "b/file"},
		{basePath: "../a/./b/", baseIsDir: true, targetPath: "../a/b/c/file", wantPath: "b/c/file"},
		{basePath: "../a/./b/", baseIsDir: true, targetPath: "../a/b/c/file", wantPath: "b/c/file"},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("base: %s (%v), target: %s -> %s", tt.basePath, tt.baseIsDir, tt.targetPath, tt.wantPath)
		t.Run(name, func(t *testing.T) {
			got, err := relPath(tt.basePath, tt.baseIsDir, tt.targetPath)
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, got)
		})
	}
}

// relTestDir is a helper to only deal with paths relative to the test directory.
// This function prepends the necessary relative path components.
func relTestDir(path string) string {
	return filepath.Join("..", "..", "test", path)
}

// tmpDir returns the directory where temporary files and directories are written.
func tmpDir() string {
	return relTestDir("tmp")
}

// tmpWriter writes the content of hdr and reader r to the test/tmp directory for comparison.
func tmpWriter(t *testing.T) func(hdr *tar.Header, r io.Reader) {
	tmpDir := relTestDir("tmp")
	require.NoError(t, os.RemoveAll(tmpDir))
	require.NoError(t, os.Mkdir(tmpDir, 0o774))

	return func(hdr *tar.Header, r io.Reader) {
		finfo := hdr.FileInfo()
		joined := filepath.Join(tmpDir, hdr.Name)
		if finfo.IsDir() {
			require.NoError(t, os.MkdirAll(joined, finfo.Mode()))
		} else {
			newFile, err := os.OpenFile(joined, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, finfo.Mode().Perm())
			require.NoError(t, err)
			_, err = io.Copy(newFile, r)
			require.NoError(t, err)
		}
	}
}

// assertTmpIntegrity checks that the source directory/file is equal to the content that is found in the content
// that was written to the tmp directory.
func assertTmpIntegrity(t *testing.T, srcObj string, srcIsDir bool) {
	tmpObj := tmpDir()
	if srcIsDir {
		tmpObj = filepath.Join(tmpObj, srcObj)
	} else {
		tmpObj = filepath.Join(tmpObj, filepath.Base(srcObj))
	}
	testObj := relTestDir(srcObj)

	type walkData struct {
		path string
		info os.FileInfo
		err  error
	}

	var tmpWalkData []walkData
	var testWalkData []walkData

	err := filepath.Walk(tmpObj, func(path string, info os.FileInfo, err error) error {
		tmpWalkData = append(tmpWalkData, walkData{path, info, err})
		return err
	})
	require.NoError(t, err)

	err = filepath.Walk(testObj, func(path string, info os.FileInfo, err error) error {
		testWalkData = append(testWalkData, walkData{path, info, err})
		return err
	})
	require.NoError(t, err)

	require.Equal(t, len(tmpWalkData), len(testWalkData))

	for i := 0; i < len(tmpWalkData); i++ {
		tmpWalk := tmpWalkData[i]
		testWalk := testWalkData[i]

		assert.NoError(t, tmpWalk.err)
		assert.NoError(t, testWalk.err)

		tmpRel, err := filepath.Rel(tmpObj, tmpWalk.path)
		require.NoError(t, err)

		testRel, err := filepath.Rel(testObj, testWalk.path)
		require.NoError(t, err)

		assert.Equal(t, tmpRel, testRel)
		assert.Equal(t, tmpWalk.info.Size(), testWalk.info.Size())
		assert.Equal(t, tmpWalk.info.Mode(), testWalk.info.Mode())
		assert.Equal(t, tmpWalk.info.IsDir(), testWalk.info.IsDir())
	}
}

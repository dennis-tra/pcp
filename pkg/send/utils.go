package send

import (
	"crypto/sha256"
	"io"
	"os"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
)

func calcContentID(filepath string) (cid.Cid, error) {

	f, err := os.Open(filepath)
	if err != nil {
		return cid.Cid{}, err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return cid.Cid{}, err
	}

	mhash, err := mh.Encode(hasher.Sum(nil), mh.SHA2_256)
	if err != nil {
		return cid.Cid{}, err
	}

	return cid.NewCidV1(cid.Raw, mhash), nil
}

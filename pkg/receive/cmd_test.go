package receive

import (
	"testing"

	"github.com/ipfs/go-cid"
)

func TestParseCID(t *testing.T) {

	var flagtests = []struct {
		str string
		cid string
	}{
		{
			"https://share.ipfs.io/#/bafybeihwouotdg5brysosbk5rhqw732t3rhzyucaxvsvbzhouwp46g5jau",
			"bafybeihwouotdg5brysosbk5rhqw732t3rhzyucaxvsvbzhouwp46g5jau",
		},
		{
			"https://share.ipfs.io/#bafybeihwouotdg5brysosbk5rhqw732t3rhzyucaxvsvbzhouwp46g5jau/",
			"bafybeihwouotdg5brysosbk5rhqw732t3rhzyucaxvsvbzhouwp46g5jau",
		},
		{
			"Qma9fv3cBwW9imNyZpEvpoovTqEAYi8ELKyPfBPVqQxuqj",
			"Qma9fv3cBwW9imNyZpEvpoovTqEAYi8ELKyPfBPVqQxuqj",
		},
		{
			"Qma9fv3cBwW0imNyZpEvpoovTqEAYi8ELKyPfBPVqQxuqj", // invalid
			"",
		},
		{
			"   bafybeihwouotdg5brysosbk5rhqw732t3rhzyucaxvsvbzhouwp46g5jau  ",
			"bafybeihwouotdg5brysosbk5rhqw732t3rhzyucaxvsvbzhouwp46g5jau",
		},
		{"https://share.ipfs.io/#/something-else", ""},
		{"some-words", ""},
		{"", ""},
	}
	for _, tt := range flagtests {
		t.Run("tt.in", func(t *testing.T) {
			c := parseCID(tt.str)
			if c.String() != tt.cid && !(c == cid.Undef && tt.cid == "") {
				t.Errorf("got cid %q, want cid %q", c.String(), tt.cid)
			}
		})
	}

}

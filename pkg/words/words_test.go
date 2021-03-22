package words

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToInts(t *testing.T) {
	testData := []struct {
		words    []string
		err      bool
		expected []int
	}{
		{words: []string{}, expected: []int{}},
		{words: []string{"abandon", "access"}, expected: []int{0, 10}},
		{words: []string{"not found"}, err: true},
		{words: []string{"access", "not found"}, err: true},
		{words: []string{"abriter", "abaisser"}, expected: []int{10, 0}},
		{words: []string{"access", "abaisser"}, err: true},
		{words: []string{"orizzonte", "bipolare", "perforare", "tacciare"}, expected: []int{1174, 235, 1258, 1805}},
	}

	for _, tt := range testData {
		t.Run(strings.Join(tt.words, "-"), func(t *testing.T) {
			res, err := ToInts(tt.words)
			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, res)
			}
		})
	}
}

func TestRandomToInts(t *testing.T) {
	langs := make([]Language, len(Lists))
	i := 0
	for lang := range Lists {
		langs[i] = lang
		i++
	}
	rand.Seed(time.Now().Unix())
	for i := 0; i < 100; i++ {
		rlang := rand.Intn(len(langs))
		rcount := 10 - rand.Intn(7)
		expInts, words, err := Random(string(langs[rlang]), rcount)
		errMsg := fmt.Sprintf("failed with lang %s and %d words", string(langs[rlang]), rcount)
		require.NoError(t, err, errMsg)
		actInts, err := ToInts(words)
		require.NoError(t, err, errMsg)
		assert.Equal(t, expInts, actInts, errMsg)
	}
}

func TestRandom_UnsupportedLanguage(t *testing.T) {
	_, _, err := Random("unsupported", 5)
	require.Error(t, err)
	assert.Equal(t, ErrUnsupportedLanguage, err)
}

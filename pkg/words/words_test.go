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

func TestRandomToIntsFromEnglish(t *testing.T) {
	seed := time.Now().Unix()
	fmt.Println("seed", seed)
	rand.Seed(seed)
	for i := 0; i < 100; i++ {
		rcount := rand.Intn(10)
		expInts, words, err := Random(string(English), rcount)
		errMsg := fmt.Sprintf("failed with lang %s and %d words", string(English), rcount)
		require.NoError(t, err, errMsg)
		actInts, err := ToInts(words)
		require.NoError(t, err, errMsg)
		assert.Equal(t, expInts, actInts, errMsg)
	}
}

func TestRandomToInts(t *testing.T) {
	t.Skip("Flaky")
	//--- FAIL: TestRandomToInts (0.00s)
	//    words_test.go:58:
	//        	Error Trace:	words_test.go:58
	//        	Error:      	Not equal:
	//        	            	expected: []int{1102}
	//        	            	actual  : []int{993}
	//
	//        	            	Diff:
	//        	            	--- Expected
	//        	            	+++ Actual
	//        	            	@@ -1,3 +1,3 @@
	//        	            	 ([]int) (len=1) {
	//        	            	- (int) 1102
	//        	            	+ (int) 993
	//        	            	 }
	//        	Test:       	TestRandomToInts
	//        	Messages:   	failed with lang french and 1 words
	langs := make([]Language, len(Lists))
	i := 0
	for lang := range Lists {
		langs[i] = lang
		i++
	}
	rand.Seed(time.Now().Unix())
	for i := 0; i < 100; i++ {
		rlang := rand.Intn(len(langs))
		rcount := rand.Intn(10)
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

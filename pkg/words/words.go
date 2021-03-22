package words

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/pkg/errors"
	"github.com/tyler-smith/go-bip39/wordlists"
)

type Language string

const (
	English            Language = "english"
	ChineseSimplified           = "chinese_simplified"
	ChineseTraditional          = "chinese_traditional"
	Czech                       = "czech"
	French                      = "french"
	Italian                     = "italian"
	Japanese                    = "japanese"
	Korean                      = "korean"
	Spanish                     = "spanish"
)

var Lists = map[Language][]string{
	English:            wordlists.English,
	ChineseSimplified:  wordlists.ChineseSimplified,
	ChineseTraditional: wordlists.ChineseTraditional,
	Czech:              wordlists.Czech,
	French:             wordlists.French,
	Italian:            wordlists.Italian,
	Japanese:           wordlists.Japanese,
	Korean:             wordlists.Korean,
	Spanish:            wordlists.Spanish,
}

var ErrUnsupportedLanguage = errors.New("unsupported language")

// Random returns a slice of random words and their respective
// integer values from the BIP39 wordlist of that given language.
func Random(lang string, count int) ([]int, []string, error) {
	wordList, err := wordsForLang(lang)
	if err != nil {
		return nil, nil, err
	}
	words := make([]string, count)
	ints := make([]int, count)
	for i := 0; i < count; i++ {
		rint, err := rand.Int(rand.Reader, big.NewInt(int64(len(wordList))))
		if err != nil {
			return nil, nil, err
		}
		words[i] = wordList[rint.Int64()]
		ints[i] = int(rint.Int64())
	}
	return ints, words, nil
}

func ToInts(words []string) ([]int, error) {
	var ints []int
ListLoop:
	for _, wordList := range Lists {
		ints = []int{}
		for _, word := range words {
			idx := wordInList(word, wordList)
			if idx == -1 {
				continue ListLoop
			}
			ints = append(ints, idx)
		}
		return ints, nil
	}
	return nil, fmt.Errorf("could not find all words in a single wordlist")
}

// HomebrewList returns a hard coded list of words, so that a full functional
// test can be carried out after a homebrew installation.
func HomebrewList() []string {
	return []string{
		wordlists.English[0],
		wordlists.English[0],
		wordlists.English[0],
		wordlists.English[0],
	}
}

// Tried sort.SearchStrings
func wordInList(word string, list []string) int {
	for i, w := range list {
		if w == word {
			return i
		}
	}
	return -1
}

func wordsForLang(lang string) ([]string, error) {
	switch Language(lang) {
	case English:
		return wordlists.English, nil
	case ChineseSimplified:
		return wordlists.ChineseSimplified, nil
	case ChineseTraditional:
		return wordlists.ChineseTraditional, nil
	case Czech:
		return wordlists.Czech, nil
	case French:
		return wordlists.French, nil
	case Italian:
		return wordlists.Italian, nil
	case Japanese:
		return wordlists.Japanese, nil
	case Korean:
		return wordlists.Korean, nil
	case Spanish:
		return wordlists.Spanish, nil
	default:
		return nil, ErrUnsupportedLanguage
	}
}

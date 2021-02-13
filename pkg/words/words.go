package words

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/tyler-smith/go-bip39/wordlists"
)

func FromBytes(b []byte) ([]string, error) {
	length := 4
	words := make([]string, length)
	for i := 0; i < length; i++ {
		sum := 0
		for j := 0; j < 8; j++ {
			sum += int(b[j+i*8+i])
		}
		words[i] = wordlists.English[sum]
	}

	return words, nil
}

func ToBytes(words []string) ([]byte, error) {
	ints, err := ToInts(words)
	if err != nil {
		return nil, err
	}
	return intsToBytes(ints)
}

func intsToBytes(ints []int16) ([]byte, error) {
	buffer := []byte{}

	for _, i := range ints {
		buf := new(bytes.Buffer)
		if err := binary.Write(buf, binary.LittleEndian, i); err != nil {
			return nil, err
		}
		buffer = append(buffer, buf.Bytes()...)
	}
	return buffer, nil
}

func ToInts(words []string) ([]int16, error) {
	ints := []int16{}
	for _, word := range words {
		i, err := ToInt(word)
		if err != nil {
			return nil, err
		}
		ints = append(ints, i)
	}
	return ints, nil
}

func ToInt(word string) (int16, error) {
	for i, w := range wordlists.English {
		if w == word {
			return int16(i), nil
		}
	}
	return 0, fmt.Errorf("word not found in list")
}

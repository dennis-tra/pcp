package wrap

import (
	"os"

	stdioutil "io/ioutil"
)

type Ioutiler interface {
	ReadFile(filename string) ([]byte, error)
	WriteFile(filename string, data []byte, perm os.FileMode) error
}

type Ioutil struct{}

func (a Ioutil) ReadFile(filename string) ([]byte, error) {
	return stdioutil.ReadFile(filename)
}

func (a Ioutil) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return stdioutil.WriteFile(filename, data, perm)
}

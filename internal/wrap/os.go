package wrap

import (
	stdos "os"
)

type Oser interface {
	ReadFile(filename string) ([]byte, error)
	WriteFile(filename string, data []byte, perm stdos.FileMode) error
}

type Os struct{}

func (Os) ReadFile(filename string) ([]byte, error) {
	return stdos.ReadFile(filename)
}

func (Os) WriteFile(filename string, data []byte, perm stdos.FileMode) error {
	return stdos.WriteFile(filename, data, perm)
}

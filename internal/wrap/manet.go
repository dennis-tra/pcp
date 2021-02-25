package wrap

import (
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

type Maneter interface {
	IsPublicAddr(a ma.Multiaddr) bool
}

type Manet struct{}

func (d Manet) IsPublicAddr(a ma.Multiaddr) bool {
	return manet.IsPublicAddr(a)
}

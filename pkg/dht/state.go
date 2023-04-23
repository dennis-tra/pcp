package dht

import (
	"fmt"

	"github.com/libp2p/go-libp2p/core/network"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

type Stage uint8

const (
	StageIdle = iota + 1
	StageBootstrapping
	StageAnalyzingNetwork
	StageWaitingForPublicAddrs
	StageProviding
	StageLookup
	StageRetrying
	StageProvided
	StageStopped
	StageError
)

func (s Stage) IsTermination() bool {
	return s == StageStopped || s == StageError
}

type AdvertiseState struct {
	Stage        Stage
	NATTypeUDP   network.NATDeviceType
	NATTypeTCP   network.NATDeviceType
	Reachability network.Reachability
	PublicAddrs  []ma.Multiaddr
	PrivateAddrs []ma.Multiaddr
	RelayAddrs   []ma.Multiaddr
	Err          error
}

func (s *AdvertiseState) String() string {
	return fmt.Sprintf("stage=%q nat_tcp=%s nat_udp=%s reachability=%s", s.Stage, s.NATTypeTCP, s.NATTypeUDP, s.Reachability)
}

func (s *AdvertiseState) populateAddrs(addrs []ma.Multiaddr) {
	s.PublicAddrs = []ma.Multiaddr{}
	s.PrivateAddrs = []ma.Multiaddr{}
	s.RelayAddrs = []ma.Multiaddr{}
	for _, addr := range addrs {
		if isRelayedMaddr(addr) { // needs to come before IsPublic because relay addrs are also public addrs
			s.RelayAddrs = append(s.RelayAddrs, addr)
		} else if manet.IsPublicAddr(addr) {
			s.PublicAddrs = append(s.PublicAddrs, addr)
		} else if manet.IsPrivateAddr(addr) {
			s.PrivateAddrs = append(s.PrivateAddrs, addr)
		}
	}
}

type DiscoverState struct {
	Stage        Stage
	PublicAddrs  []ma.Multiaddr
	PrivateAddrs []ma.Multiaddr
	Err          error
}

// TODO remove duplication
func (s *DiscoverState) populateAddrs(addrs []ma.Multiaddr) {
	s.PublicAddrs = []ma.Multiaddr{}
	s.PrivateAddrs = []ma.Multiaddr{}
	for _, addr := range addrs {
		if manet.IsPublicAddr(addr) {
			s.PublicAddrs = append(s.PublicAddrs, addr)
		} else if manet.IsPrivateAddr(addr) {
			s.PrivateAddrs = append(s.PrivateAddrs, addr)
		}
	}
}

func isRelayedMaddr(maddr ma.Multiaddr) bool {
	_, err := maddr.ValueForProtocol(ma.P_CIRCUIT)
	return err == nil
}

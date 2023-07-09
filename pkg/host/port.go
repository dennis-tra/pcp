package host

import (
	"fmt"

	ma "github.com/multiformats/go-multiaddr"
)

type portMapping struct {
	internal string
	external string
	network  string
}

func (pm portMapping) String() string {
	return fmt.Sprintf("%s -> %s (%s)", pm.internal, pm.external, pm.network)
}

func (m *Model) portMappings() map[string]portMapping {
	mappings := map[string]portMapping{}
	for _, maddr := range m.PrivateAddrs {
		mapping := m.NATManager.GetMapping(maddr)
		if mapping == nil {
			continue
		}

		intPort, intNet, err := extractPort(maddr)
		if err != nil {
			continue
		}

		extPort, extNet, err := extractPort(mapping)
		if err != nil {
			continue
		}

		if intNet != extNet {
			continue
		}

		pm := portMapping{
			internal: intPort,
			external: extPort,
			network:  intNet,
		}

		mappings[pm.String()] = pm
	}

	return mappings
}

func extractPort(m ma.Multiaddr) (string, string, error) {
	network := "tcp"
	port, err := m.ValueForProtocol(ma.P_TCP)
	if err != nil {
		network = "udp"
		port, err = m.ValueForProtocol(ma.P_UDP)
		if err != nil {
			return "", "", nil
		}
	}
	return port, network, nil
}

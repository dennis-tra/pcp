package initialize

import (
	"github.com/dennis-tra/pcp/internal/log"
	"github.com/urfave/cli/v2"

	"github.com/dennis-tra/pcp/pkg/config"
)

// Command contains the initialization logic of pcp.
var Command = &cli.Command{
	Name:        "init",
	Usage:       "initializes pcp by e.g. create an identity",
	Aliases:     []string{"i"},
	Action:      Action,
	ArgsUsage:   "",
	UsageText:   ``,
	Description: ``,
}

// Action is the function that is called when running pcp receive.
func Action(c *cli.Context) error {

	conf, err := config.LoadConfig()
	if err != nil {
		return err
	}

	if conf.Settings.Exists {
		log.Infoln("Loaded settings.json from: ", conf.Settings.Path)
	}

	if conf.Identity.Exists {
		log.Infoln("Loaded identity.json from: ", conf.Identity.Path)
	}

	if !conf.Identity.IsInitialized() {
		err := conf.Identity.GenerateKeyPair()
		if err != nil {
			return err
		}
	}

	return conf.Save()
}

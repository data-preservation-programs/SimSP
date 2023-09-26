package main

import (
	"os"

	"github.com/filecoin-project/go-address"
	log "github.com/ipfs/go-log/v2"
	"github.com/urfave/cli/v2"
)

func init() {
	if os.Getenv("GOLOG_LOG_LEVEL") == "" {
		os.Setenv("GOLOG_LOG_LEVEL", "info")
	}
	address.CurrentNetwork = address.Mainnet
}

func main() {
	if log.GetConfig().Level > log.LevelInfo && os.Getenv("GOLOG_LOG_LEVEL") == "info" {
		log.SetAllLoggers(log.LevelInfo)
	}
	app := &cli.App{
		Name:  "sim-sp",
		Usage: "Utility for simulating a storage provider",
		Commands: []*cli.Command{
			startCmd,
			generatePeerCmd,
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		panic(err)
	}
}

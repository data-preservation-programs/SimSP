package main

import (
	"context"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "sim-sp",
		Usage: "Utility for simulating a storage provider",
		Commands: []*cli.Command{
			{
				Name:  "run",
				Usage: "Run the simulated storage provider",
				Action: func(c *cli.Context) error {
					return start(c.Context)
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		panic(err)
	}
}

func start(ctx context.Context) error {
	return nil
}

package main

import (
	"os"

	"github.com/TeaMeow/KitSvc/version"
	"github.com/codegangsta/cli"
)

func main() {
	serverReady := make(chan bool)

	app := cli.NewApp()
	app.Name = "service"
	app.Version = version.Version
	app.Usage = "starts the service daemon."
	app.Action = func(c *cli.Context) {
		server(c, serverReady)
	}
	app.Flags = serverFlags

	app.Run(os.Args)
}

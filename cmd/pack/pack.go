package main

import (
	"fmt"
	"github.com/codegangsta/cli"
	"os"
	"strings"

	"github.com/docker/libpack"
)

func main() {
	app := cli.NewApp()
	app.Name = "pack"
	app.Usage = "A simple command-line interface to libpack"
	app.Version = "0.0.1"
	app.Flags = []cli.Flag{}
	app.Commands = []cli.Command{
		{
			Name:	"unpack",
			Usage:	"",
			Action:	cmdUnpack,
		},
		{
			Name:	"pack",
			Usage:	"",
			Action:	cmdPack,
		},
	}
	app.Run(os.Args)
}

func cmdUnpack(c *cli.Context) {
	if !c.Args().Present() {
		Fatalf("usage: unpack HASH")
	}
	if err := libpack.Unpack(".git", ".", c.Args()[0]); err != nil {
		Fatalf("unpack: %v", err)
	}
}

func cmdPack(c *cli.Context) {
	if len(c.Args()) != 1 {
		Fatalf("usage: pack BRANCH")
	}
	hash, err := libpack.Pack(".git", ".", c.Args()[0])
	if err != nil {
		Fatalf("unpack: %v", err)
	}
	fmt.Println(hash)
}

func Fatalf(msg string, args ...interface{}) {
	if !strings.HasSuffix(msg, "\n") {
		msg = msg + "\n"
	}
	fmt.Fprintf(os.Stderr, msg, args...)
	os.Exit(1)
}

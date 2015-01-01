package main

import (
	"fmt"
	"log"
	"os"

	"github.com/docker/libpack"
)

func main() {
	fmt.Printf("Opening repository at %s\n", "pack.db")
	repo, err := libpack.Init("pack.db", true)
	if err != nil {
		log.Fatal(err)
	}

	var cmd string
	if len(os.Args) >= 2 {
		cmd = os.Args[1]
	} else {
		cmd = "serve"
	}

	switch cmd {
	case "serve":
		{
			fmt.Printf("Listening on tcp://0.0.0.0:4242\n")
			if err := repo.ListenAndServe("tcp", "0.0.0.0:4242"); err != nil {
				log.Fatal(err)
			}
		}
	case "query":
		{
			p := libpack.NewPipeline(repo)
			if err := p.Communicate(os.Stdin, os.Stdout, os.Stderr); err != nil {
				log.Fatal(err)
			}
		}
	}
}

package main

import (
	"fmt"
	"log"

	"github.com/docker/libpack"
)

func main() {
	fmt.Printf("Opening repository at %s\n", "pack.db")
	repo, err := libpack.Init("pack.db", true)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Listening on tcp://0.0.0.0:4242\n")
	if err := repo.ListenAndServe("tcp", "0.0.0.0:4242"); err != nil {
		log.Fatal(err)
	}
}

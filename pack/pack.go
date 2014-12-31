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

	fmt.Printf("Opening DB '%s'\n", "refs/heads/db")
	db, err := repo.DB("refs/heads/db")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Generating keypair\n")
	key, err := libpack.GenerateKey()
	if err != nil {
		log.Fatal(err)
	}

	srv := libpack.NewServer(key, db)

	fmt.Printf("Listening on tcp://0.0.0.0:4242\n")
	if err := srv.ListenAndServe("tcp", "0.0.0.0:4242"); err != nil {
		log.Fatal(err)
	}
}

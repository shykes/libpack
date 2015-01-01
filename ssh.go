package libpack

import (
	"fmt"
	"io"
	"strings"

	"code.google.com/p/go.crypto/ssh"
	"github.com/docker/libpack/pkg/dssh"
)

// Repository implements dssh.Server, and dssh.SessionHandler, awesome!

func (r *Repository) HandleSSHSubsystem(name string, stdin io.Reader, stdout, stderr io.Writer, reqs <-chan *ssh.Request) error {
	if name == "query-0.0.1@sandbox.docker.io" {
		go ssh.DiscardRequests(reqs)
		p := NewPipeline(r)
		return p.Communicate(stdin, stdout, stderr)
	}
	return dssh.Unsupported
}

func (r *Repository) HandleSSHExec(arg string, stdin io.Reader, stdout, stderr io.Writer, reqs <-chan *ssh.Request) error {
	go ssh.DiscardRequests(reqs)
	words := strings.Split(arg, " ")
	if len(words) == 0 {
		return fmt.Errorf("no arguments")
	}
	var (
		cmd  = words[0]
		args = words[1:]
	)
	switch cmd {
	case "get":
		{
			if len(args) != 2 {
				return fmt.Errorf("usage: get DB KEY")
			}
			db, err := r.DB(args[0])
			if err != nil {
				return err
			}
			value, err := db.Get(args[1])
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "%s\n", value)
		}
	case "set":
		{
			if len(args) != 3 {
				return fmt.Errorf("usage: set DB KEY VALUE")
			}
			db, err := r.DB(args[0])
			if err != nil {
				return err
			}
			result, err := db.Set(args[1], args[2])
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "%s\n", result.Hash())
		}
	case "list":
		{
			if len(args) != 2 {
				return fmt.Errorf("usage: list DB KEY")
			}
			db, err := r.DB(args[0])
			if err != nil {
				return err
			}
			names, err := db.List(args[1])
			if err != nil {
				return err
			}
			for _, name := range names {
				fmt.Fprint(stdout, name)
			}
		}
	case "dump":
		{
			if len(args) != 1 {
				return fmt.Errorf("usage: dump DB")
			}
			db, err := r.DB(args[0])
			if err != nil {
				return err
			}
			if err := db.Dump(stdout); err != nil {
				return err
			}
		}
	case "hash":
		{
			if len(args) != 1 {
				return fmt.Errorf("usage: hash DB")
			}
			db, err := r.DB(args[0])
			if err != nil {
				return err
			}
			hash, err := db.Hash()
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "%s\n", hash)
		}
	case "ping":
		{
			fmt.Fprintf(stdout, "pong\n")
		}
	default:
		{
			return fmt.Errorf("exec: no such command: %s", cmd)
		}
	}
	return nil
}

func (r *Repository) HandleSSHShell(arg string, stdin io.Reader, stdout, stderr io.Writer, reqs <-chan *ssh.Request) error {
	return dssh.Unsupported
}

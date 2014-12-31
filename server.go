package libpack

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"strings"

	"code.google.com/p/go.crypto/ssh"
	"github.com/docker/libtrust"
)

type SSHHandler interface {
	AcceptSSH(chType, chArg string) bool
	HandleSSH(chType, chArg string, ch ssh.Channel, reqs <-chan *ssh.Request)
}

func GenerateKey() (ssh.Signer, error) {
	pk, err := libtrust.GenerateECP521PrivateKey()
	if err != nil {
		return nil, err
	}
	s, err := ssh.NewSignerFromKey(pk.CryptoPrivateKey())
	if err != nil {
		return nil, err
	}
	return s, nil
}

type Server struct {
	sshCfg *ssh.ServerConfig
	h      SSHHandler
}

func NewServer(key ssh.Signer, h SSHHandler) *Server {
	sshCfg := &ssh.ServerConfig{
		// PublicKeyCallback: allowAll,
		NoClientAuth: true,
	}
	sshCfg.AddHostKey(key)
	srv := &Server{
		sshCfg: sshCfg,
		h:      h,
	}
	return srv
}

func (srv *Server) ListenAndServe(proto, addr string) error {
	l, err := net.Listen(proto, addr)
	if err != nil {
		return err
	}
	return srv.Serve(l)
}

func (srv *Server) Serve(l net.Listener) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return fmt.Errorf("accept: %v", err)
		}
		go srv.ServeConn(conn)
	}
	return nil
}

func (srv *Server) ServeConn(conn net.Conn) error {
	_, chans, reqs, err := ssh.NewServerConn(conn, srv.sshCfg)
	if err != nil {
		return fmt.Errorf("handshake: %v", err)
	}
	go ssh.DiscardRequests(reqs)
	for nch := range chans {
		var (
			chType = nch.ChannelType()
			chArg  = string(nch.ExtraData())
		)
		if !srv.h.AcceptSSH(chType, chArg) {
			nch.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		ch, reqs, err := nch.Accept()
		if err != nil {
			return fmt.Errorf("accept: %v", err)
		}
		// FIXME: use context.Context to cleanly synchronize with handlers, block on them
		// but still be able to terminate them gracefully.
		go func(ch ssh.Channel, reqs <-chan *ssh.Request) {
			srv.h.HandleSSH(chType, chArg, ch, reqs)
			ch.Close()
		}(ch, reqs)
	}
	return nil
}

// DB implements SSHHandler, awesome!

func (db *DB) AcceptSSH(chType, chArg string) bool {
	return chType == "session"
}

func (db *DB) HandleSSH(chType, chArg string, ch ssh.Channel, reqs <-chan *ssh.Request) {
	if chType != "session" {
		// unsupported channel type
		return
	}
	var (
		h   func(ssh.Channel, *ssh.Request) error
		req *ssh.Request
	)
reqLoop:
	for req = range reqs {
		switch req.Type {
		// All 3 main session requests (shell, subsystem, exec)
		case "shell":
			{
				h = db.handleShell
				break reqLoop
			}
		case "subsystem":
			{
				h = db.handleSubsystem
				break reqLoop
			}
		case "exec":
			{
				h = db.handleExec
				break reqLoop
			}
		default:
			fmt.Fprintf(ch.Stderr(), "unsupported channel request: %s\n", req.Type)
			req.Reply(false, nil)
		}
	}
	if h != nil {
		go ssh.DiscardRequests(reqs)
		err := h(ch, req)
		if err != nil {
			log.Printf("channel handler returned an error: %v\n", err)
			fmt.Fprintf(ch.Stderr(), "%v\n", err)
			req.Reply(false, nil)
		} else {
			req.Reply(true, nil)
		}
	}
}

func (db *DB) handleShell(ch ssh.Channel, req *ssh.Request) error {
	return fmt.Errorf("FIXME: shell not implemented")
}

func (db *DB) handleExec(ch ssh.Channel, req *ssh.Request) error {
	words := strings.Split(string(req.Payload[4:]), " ")
	if len(words) == 0 {
		return fmt.Errorf("no arguments")
	}
	var (
		cmd    = words[0]
		args   = words[1:]
		stdout = ch
	)
	switch cmd {
	case "get":
		{
			if len(args) != 1 {
				return fmt.Errorf("usage: get VALUE")
			}
			value, err := db.Get(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "%s", value)
		}
	case "set":
		{
			if len(args) != 2 {
				return fmt.Errorf("usage: set KEY VALUE")
			}
			_, err := db.Set(args[0], args[1])
			if err != nil {
				return err
			}
		}
	case "list":
		{
			if len(args) != 1 {
				return fmt.Errorf("usage: list KEY")
			}
			names, err := db.List(args[0])
			if err != nil {
				return err
			}
			for _, name := range names {
				fmt.Fprint(stdout, name)
			}
		}
	case "dump":
		{
			if len(args) != 0 {
				return fmt.Errorf("usage: dump")
			}
			err := db.Dump(stdout)
			if err != nil {
				return err
			}
		}
	case "hash":
		{
			if len(args) != 0 {
				return fmt.Errorf("usage: hash")
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

func (db *DB) handleSubsystem(ch ssh.Channel, req *ssh.Request) error {
	name := string(req.Payload[4:])
	if name == "mirror-0.0.1@sandbox.docker.io" {
		_, err := io.Copy(ch, ch)
		return err
	} else if name == "query-0.0.1@sandbox.docker.io" {
		return db.serveQuery(ch, ch, ch.Stderr())
	}
	return fmt.Errorf("unsupported subsystem: %s", name)
}

type Command struct {
	Op   string
	Args []string
}

func (db *DB) serveQuery(in io.Reader, out io.Writer, stderr io.Writer) error {
	jin := json.NewDecoder(in)
	jout := json.NewEncoder(out)
	p := NewPipeline(db.Repo())
	for {
		var cmd Command
		if err := jin.Decode(&cmd); err != nil {
			return err
		}
		fmt.Printf("--> OP = '%s' ARGS = '%v'\n", cmd.Op, cmd.Args)
		switch cmd.Op {
		case "query":
			{
				p.Query(db)
			}
		case "commit":
			{
				p.Commit(db)
			}
		case "run":
			{
				result, err := p.Run()
				if err != nil {
					return jout.Encode(&Command{Op: "error", Args: []string{err.Error()}})
				}
				return jout.Encode(&Command{Op: "sethash", Args: []string{result.Hash()}})
			}
		case "dump":
			{
				p.Dump(stderr)
			}
		case "scope":
			{
				if len(cmd.Args) != 1 {
					return jout.Encode(&Command{Op: "error", Args: []string{"Usage: scope KEY"}})
				}
				p.Scope(cmd.Args[0])
			}
		default:
			return jout.Encode(&Command{Op: "error", Args: []string{"no such command", cmd.Op}})
		}
	}
	return nil
}

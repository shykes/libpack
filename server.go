package libpack

import (
	"fmt"
	"io"
	"net"
	"strings"

	"code.google.com/p/go.crypto/ssh"
	"github.com/docker/libtrust"
)

type SSHHandler interface {
	AcceptSSH(chType, chArg string) bool
	HandleSSH(chType, chArg string, ch ssh.Channel, reqs <-chan *ssh.Request) error
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
			if err := srv.h.HandleSSH(chType, chArg, ch, reqs); err != nil {
				fmt.Fprintf(ch.Stderr(), "--> %v\n", err)
			}
			ch.Close()
		}(ch, reqs)
	}
	return nil
}

// DB implements SSHHandler, awesome!

func (db *DB) AcceptSSH(chType, chArg string) bool {
	if chType == "session" {
		return true
	}
	return false
}

func (db *DB) HandleSSH(chType, chArg string, ch ssh.Channel, reqs <-chan *ssh.Request) error {
	if chType == "session" {
		return db.handleSSHSession(ch, reqs)
	}
	return fmt.Errorf("unsupported channel type: %s", chType)
}

func (db *DB) handleSSHSession(ch ssh.Channel, reqs <-chan *ssh.Request) error {
	for req := range reqs {
		switch req.Type {
		case "shell":
			{
				return fmt.Errorf("FIXME: shell not implemented")
			}
		case "exec":
			{
				words := strings.Split(string(req.Payload[4:]), " ")
				if len(words) == 0 {
					return fmt.Errorf("no arguments")
				}
				handleExec := func(cmd string, args []string, stdout io.Writer) error {
					defer ch.Close()
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
				return handleExec(words[0], words[1:], ch)
			}
		default:
			return fmt.Errorf("unsupported session command")
		}
	}
	return nil
}

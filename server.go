package libpack

import (
	"fmt"
	"io"
	"net"
	"strings"

	"code.google.com/p/go.crypto/ssh"
	"github.com/docker/libtrust"
)

type Server struct {
	sshCfg *ssh.ServerConfig
	h      Handler
}

type Handler interface {
	Get(key string) (string, error)
	Set(key, value string) (*Tree, error)
	List(key string) ([]string, error)
	Dump(dst io.Writer) error
	Hash() (string, error)
}

func NewServer(key ssh.Signer, h Handler) *Server {
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

type Cmd struct {
	Name   string
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
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

func allowAll(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
	fmt.Printf("Public key request:\n--> %v\n--> %v\n", conn, key)
	return nil, nil
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
	// Before use, a handshake must be performed on the incoming
	// net.Conn.
	_, chans, reqs, err := ssh.NewServerConn(conn, srv.sshCfg)
	if err != nil {
		return fmt.Errorf("handshake: %v", err)
	}
	// The incoming Request channel must be serviced.
	go ssh.DiscardRequests(reqs)

	// Service the incoming Channel channel.
	for newChannel := range chans {
		// Channels have a type, depending on the application level
		// protocol intended. In the case of a shell, the type is
		// "session" and ServerShell may be used to present a simple
		// terminal interface.
		// fmt.Printf("--> NEWCHAN '%s' '%s'\n", newChannel.ChannelType(), newChannel.ExtraData())
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			return fmt.Errorf("could not accept channel.")
		}

		go srv.serveChannel(channel, requests)
	}
	return nil
}

func (srv *Server) handleChannelRequest(channel ssh.Channel, req *ssh.Request) error {
	switch req.Type {
	case "exec":
		{
			defer channel.Close()
			args := strings.Split(string(req.Payload[4:]), " ")
			if len(args) == 0 {
				return fmt.Errorf("no arguments")
			}
			return srv.HandleCommand(&Cmd{
				Name:   args[0],
				Args:   args[1:],
				Stdin:  channel,
				Stdout: channel,
				Stderr: channel.Stderr(),
			})
		}
	default:
		return fmt.Errorf("unsupported channel request")
	}
}

func (srv *Server) HandleCommand(cmd *Cmd) (err error) {
	fmt.Printf("CMD: [%s] [%v]\n", cmd.Name, cmd.Args)
	defer func() {
		if err != nil {
			fmt.Fprintf(cmd.Stderr, "--> %v\n", err)
		}
	}()
	switch cmd.Name {
	case "get":
		{
			if len(cmd.Args) != 1 {
				return fmt.Errorf("usage: get VALUE")
			}
			value, err := srv.h.Get(cmd.Args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.Stdout, "%s", value)
		}
	case "set":
		{
			if len(cmd.Args) != 2 {
				return fmt.Errorf("usage: set KEY VALUE")
			}
			_, err := srv.h.Set(cmd.Args[0], cmd.Args[1])
			if err != nil {
				return err
			}
		}
	case "list":
		{
			if len(cmd.Args) != 1 {
				return fmt.Errorf("usage: list KEY")
			}
			names, err := srv.h.List(cmd.Args[0])
			if err != nil {
				return err
			}
			for _, name := range names {
				fmt.Fprint(cmd.Stdout, name)
			}
		}
	case "dump":
		{
			if len(cmd.Args) != 0 {
				return fmt.Errorf("usage: dump")
			}
			err := srv.h.Dump(cmd.Stdout)
			if err != nil {
				return err
			}
		}
	case "hash":
		{
			if len(cmd.Args) != 0 {
				return fmt.Errorf("usage: hash")
			}
			hash, err := srv.h.Hash()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.Stdout, "%s\n", hash)
		}
	case "ping":
		{
			fmt.Fprintf(cmd.Stdout, "pong\n")
		}
	default:
		{
			return fmt.Errorf("unsupport command: %s", cmd.Name)
		}
	}
	return nil
}

func (srv *Server) serveChannel(channel ssh.Channel, requests <-chan *ssh.Request) {
	for req := range requests {
		if err := srv.handleChannelRequest(channel, req); err != nil {
			req.Reply(false, nil)
		} else {
			req.Reply(true, nil)
		}
	}
}

package tunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
)

type SSHConfig struct {
	User       string
	Host       string
	Port       int
	PrivateKey []byte

	RemoteHost string
	RemotePort int
}

type SSH struct {
	localServer net.Listener
	sshClient   *ssh.Client
	remoteAddr  string

	err error

	cancel context.CancelFunc
	ctx    context.Context

	backgroundWG sync.WaitGroup
}

func Listen(config *SSHConfig) (*SSH, error) {
	singer, err := ssh.ParsePrivateKey(config.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %s", err.Error())
	}

	endpoint := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	sshClient, err := ssh.Dial("tcp", endpoint, &ssh.ClientConfig{
		User: config.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(singer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		BannerCallback:  ssh.BannerDisplayStderr(),
		Timeout:         10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("server %q dial error: %w", endpoint, err)
	}

	listener, err := net.Listen("tcp", `127.0.0.1:0`)
	if err != nil {
		sshClient.Close()
		return nil, fmt.Errorf("listening on %s://%s: %w", "tcp", listener.Addr().String(), err)
	}

	bgCtx, bgCancel := context.WithCancel(context.Background())

	tunnel := &SSH{
		localServer: listener,
		sshClient:   sshClient,
		remoteAddr: net.JoinHostPort(
			config.RemoteHost,
			strconv.Itoa(config.RemotePort),
		),
		ctx:    bgCtx,
		cancel: bgCancel,
	}

	go tunnel.listen()

	return tunnel, nil
}

func (t *SSH) Addr() string {
	return t.localServer.Addr().String()
}

func (t *SSH) Error() error {
	return t.err
}

func (tunnel *SSH) Close() error {
	_ = tunnel.localServer.Close()
	tunnel.cancel()
	err := tunnel.sshClient.Close()
	tunnel.backgroundWG.Wait()
	return err
}

func (t *SSH) listen() {
	t.backgroundWG.Add(1)
	defer t.backgroundWG.Done()

	for {
		conn, err := t.localServer.Accept()
		if err != nil {
			fmt.Printf("accepting connection errored out: %s", err.Error())
			return
		}

		t.backgroundWG.Add(1)
		go func() {
			defer t.backgroundWG.Done()

			t.err = t.forward(conn)
		}()
	}
}

func (t *SSH) forward(localConn net.Conn) error {
	defer localConn.Close()

	remoteConn, err := t.sshClient.Dial("tcp", t.remoteAddr)
	if err != nil {
		return err
	}

	g := errgroup.Group{}
	g.Go(func() error {
		_, err = io.Copy(remoteConn, localConn)
		if err != nil {
			return err
		}
		return nil
	})
	g.Go(func() error {
		_, err = io.Copy(localConn, remoteConn)
		if err != nil {
			return err
		}
		return nil
	})

	<-t.ctx.Done()
	remoteConn.Close()

	return g.Wait()
}

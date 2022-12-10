package tunnel_test

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/phayes/freeport"
	"github.com/rudderlabs/sql-tunnels/tunnel"
	"github.com/rudderlabs/sql-tunnels/tunnel/testhelper"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/sync/errgroup"
)

type echoServer struct {
	net.Listener

	openConn  atomic.Int64
	closeConn atomic.Int64
}

func (e *echoServer) Listen(t *testing.T) {
	listen, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	g := errgroup.Group{}

	t.Cleanup(func() {
		err = listen.Close()
		require.NoError(t, err)
		err = g.Wait()
		require.NoError(t, err)
	})

	e.Listener = listen

	g.Go(func() error {
		for {
			conn, err := listen.Accept()
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			if err != nil {
				t.Log("listen error:", err)
				return err
			}

			t.Log("new connection", conn.RemoteAddr().String())
			e.openConn.Add(1)

			g.Go(func() error {
				_, err = io.Copy(conn, conn)

				t.Log("end connection", conn.RemoteAddr().String())
				e.closeConn.Add(1)
				require.NoError(t, err)

				return nil
			})
		}
	})
}

func Test_SSH_ListenAndForward(t *testing.T) {
	t.Run("invalid private key", func(t *testing.T) {
		privateKey := []byte("invalid")
		config := tunnel.SSHConfig{
			PrivateKey: privateKey,
		}

		sshTunnel, err := tunnel.Listen(&config)
		require.EqualError(t, err, "parsing private key: ssh: no key found")
		require.Nil(t, sshTunnel)
	})

	t.Run("ssh server not available", func(t *testing.T) {
		privateKey, _ := testhelper.SSHKeyPairs(t)

		port := freeport.GetPort()

		config := tunnel.SSHConfig{
			User:       "any",
			Host:       "127.0.0.1",
			Port:       port,
			PrivateKey: privateKey,
		}

		sshTunnel, err := tunnel.Listen(&config)

		require.EqualError(t, err, fmt.Sprintf("server \"127.0.0.1:%d\" dial error: dial tcp 127.0.0.1:%d: connect: connection refused", port, port))
		require.Nil(t, sshTunnel)
	})

	t.Run("remote endpoint not available", func(t *testing.T) {
		privateKey, publicKey := testhelper.SSHKeyPairs(t)
		sshPort := testhelper.SSHServer(t, publicKey)

		port := freeport.GetPort()

		config := tunnel.SSHConfig{
			User:       "any",
			Host:       "localhost",
			Port:       sshPort,
			PrivateKey: privateKey,

			RemoteHost: "localhost",
			RemotePort: port,
		}

		sshTunnel, err := tunnel.Listen(&config)
		require.NoError(t, err)

		t.Cleanup(func() {
			err := sshTunnel.Close()
			require.NoError(t, err)
		})

		defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

		conn, err := net.Dial("tcp", sshTunnel.Addr())
		require.NoError(t, err)
		t.Cleanup(func() {
			err = conn.Close()
			require.NoError(t, err)
		})

		_, err = conn.Write([]byte("hello"))
		require.NoError(t, err)

		buf := make([]byte, 5)
		_, err = conn.Read(buf)
		require.ErrorContains(t, err, "read: connection reset by peer")

		err = sshTunnel.Error()
		require.ErrorContains(t, err, "ssh: rejected: connect failed")
		require.ErrorContains(t, err, "connect: connection refused")
	})

	t.Run("successful connection", func(t *testing.T) {
		privateKey, publicKey := testhelper.SSHKeyPairs(t)
		sshPort := testhelper.SSHServer(t, publicKey)

		echo := &echoServer{}
		echo.Listen(t)

		config := tunnel.SSHConfig{
			User:       "any",
			Host:       "localhost",
			Port:       sshPort,
			PrivateKey: privateKey,
			RemoteHost: "localhost",
			RemotePort: echo.Addr().(*net.TCPAddr).Port,
		}

		sshTunnel, err := tunnel.Listen(&config)
		require.NoError(t, err)

		t.Cleanup(func() {
			err := sshTunnel.Close()
			require.NoError(t, err)
		})

		defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
		t.Log("multiple connections successfully forwarded and closed")

		for i := 1; i < 10; i++ {
			conn, err := net.Dial("tcp", sshTunnel.Addr())
			require.NoError(t, err)

			_, err = conn.Write([]byte("hello"))
			require.NoError(t, err)

			buf := make([]byte, 5)
			_, err = conn.Read(buf)
			require.NoError(t, err)
			require.Equal(t, "hello", string(buf))

			require.Equal(t, int64(i), echo.openConn.Load())

			err = conn.Close()
			require.NoError(t, err)

			t.Log("wait for connection to eventually close")
			require.Eventually(t, func() bool {
				return echo.closeConn.Load() == int64(i)
			}, time.Second, time.Millisecond)

			require.Equal(t, echo.closeConn.Load(), int64(i))

			require.NoError(t, sshTunnel.Error())
		}
	})
}

func Test_SSH_ListenAndForward_NetworkFailure(t *testing.T) {
	privateKey, publicKey := testhelper.SSHKeyPairs(t)
	sshPort := testhelper.SSHServer(t, publicKey)

	echo := &echoServer{}
	echo.Listen(t)

	config := tunnel.SSHConfig{
		User:       "any",
		Host:       "localhost",
		Port:       sshPort,
		PrivateKey: privateKey,
		RemoteHost: "localhost",
		RemotePort: echo.Addr().(*net.TCPAddr).Port,
	}

	sshTunnel, err := tunnel.Listen(&config)
	require.NoError(t, err)

	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	conn, err := net.Dial("tcp", sshTunnel.Addr())
	require.NoError(t, err)

	err = helloWorld(conn)
	require.NoError(t, err)

	err = sshTunnel.Close()
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return helloWorld(conn) != nil
	}, time.Second, time.Millisecond)

	require.Error(t, sshTunnel.Error())
}

func helloWorld(conn net.Conn) error {
	_, err := conn.Write([]byte("hello"))
	if err != nil {
		return err
	}

	buf := make([]byte, 5)
	_, err = conn.Read(buf)
	if err != nil {
		return err
	}

	return nil

}

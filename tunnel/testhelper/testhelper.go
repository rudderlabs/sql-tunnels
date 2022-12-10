package testhelper

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"testing"

	sshx "github.com/gliderlabs/ssh"
	"github.com/phayes/freeport"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// SSHKeyPairs generates a private and public key pair
func SSHKeyPairs(t *testing.T) ([]byte, []byte) {
	t.Helper()

	reader := rand.Reader
	bitSize := 2048

	privateKey, err := rsa.GenerateKey(reader, bitSize)
	require.NoError(t, err)

	err = privateKey.Validate()
	require.NoError(t, err)

	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)

	// Get ASN.1 DER format
	privDER := x509.MarshalPKCS1PrivateKey(privateKey)

	// pem.Block
	privBlock := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privDER,
	}

	// Private key in PEM format
	privatePEM := pem.EncodeToMemory(&privBlock)

	pubKeyBytes := ssh.MarshalAuthorizedKey(publicKey)

	return privatePEM, pubKeyBytes
}

func SSHServer(t *testing.T, publicAuthorizedKey []byte) int {
	t.Helper()

	port := freeport.GetPort()

	publicKey, _, _, _, err := sshx.ParseAuthorizedKey(publicAuthorizedKey)
	require.NoError(t, err)

	server := sshx.Server{
		LocalPortForwardingCallback: sshx.LocalPortForwardingCallback(func(ctx sshx.Context, dhost string, dport uint32) bool {
			t.Log("Accepted forward", dhost, dport)
			return true
		}),
		Addr: fmt.Sprintf("127.0.0.1:%d", port),
		Handler: sshx.Handler(func(s sshx.Session) {
			_, _ = io.WriteString(s, "Remote forwarding available...\n")
			select {}
		}),
		ReversePortForwardingCallback: sshx.ReversePortForwardingCallback(func(ctx sshx.Context, host string, port uint32) bool {
			t.Log("attempt to bind", host, port, "granted")
			return true
		}),
		ChannelHandlers: map[string]sshx.ChannelHandler{
			"session":      sshx.DefaultSessionHandler,
			"direct-tcpip": sshx.DirectTCPIPHandler,
		},
		PublicKeyHandler: func(ctx sshx.Context, key sshx.PublicKey) bool {
			return sshx.KeysEqual(key, publicKey)
		},
	}

	wait := make(chan struct{})
	t.Cleanup(func() {
		err := server.Close()
		if err != nil {
			t.Log("shutdown ssh: ", err)
		}
		<-wait
	})

	go func() {
		err := server.ListenAndServe()
		require.Equal(t, sshx.ErrServerClosed, err)
		close(wait)
	}()

	return port
}

package ssh_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	sshdriver "github.com/rudderlabs/sql-tunnels/driver/ssh"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/rudderlabs/compose-test/testcompose"
)

func pingSSH(t *testing.T, dest string, config sshdriver.Config) error {
	t.Helper()

	singer, err := ssh.ParsePrivateKey(config.PrivateKey)
	if err != nil {
		return fmt.Errorf("test: parsing private key: %s", err.Error())
	}

	client, err := ssh.Dial(
		"tcp",
		fmt.Sprintf("%s:%d", config.Host, config.Port),
		&ssh.ClientConfig{
			User:            config.User,
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(singer)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			BannerCallback:  ssh.BannerDisplayStderr(),
		},
	)
	if err != nil {
		return fmt.Errorf("test: ssh dialing: %s", err.Error())
	}
	defer client.Close()

	t.Log(client.ServerVersion())

	conn, err := client.Dial("tcp", dest)
	if err != nil {
		return fmt.Errorf("test: connection %q dialing: %s", dest, err.Error())
	}
	defer conn.Close()

	return nil
}

func TestConnections(t *testing.T) {
	t.Parallel()

	privateKey, err := os.ReadFile("testdata/test_key")
	require.Nil(t, err)

	c := testcompose.New(t, "./testdata/docker-compose.yaml")

	t.Cleanup(func() {
		c.Stop(context.Background())
	})
	c.Start(context.Background())

	config := sshdriver.Config{
		User:       c.Env("openssh-server", "USER_NAME"),
		Host:       "0.0.0.0",
		Port:       c.Port("openssh-server", 2222),
		PrivateKey: privateKey,
	}

	t.Run("postgres", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		encoded, err := config.EncodeWithDSN("postgres://postgres:postgres@db_postgres:5432/postgres?sslmode=disable")
		require.NoError(t, err)

		db, err := sql.Open("sql+ssh", encoded)
		require.NoError(t, err)

		err = db.PingContext(ctx)
		require.NoError(t, err)

		err = db.Close()
		require.NoError(t, err)
	})

	t.Run("clickhouse", func(t *testing.T) {
		// FIXME: hack to wait for clickhouse to be ready
		require.Eventually(t, func() bool {
			return pingSSH(t, "db_clickhouse:9000", config) == nil
		}, time.Minute, time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		encoded, err := config.EncodeWithDSN("clickhouse://clickhouse:clickhouse@db_clickhouse:9000/db?skip_verify=false")
		require.NoError(t, err)

		db, err := sql.Open("sql+ssh", encoded)
		require.NoError(t, err)

		err = db.PingContext(ctx)
		require.NoError(t, err)

		err = db.Close()
		require.NoError(t, err)
	})
}

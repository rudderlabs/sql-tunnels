package ssh_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"

	"github.com/rudderlabs/rudder-go-kit/testhelper/docker/resource/clickhouse"
	"github.com/rudderlabs/rudder-go-kit/testhelper/docker/resource/postgres"
	"github.com/rudderlabs/rudder-go-kit/testhelper/docker/resource/sshserver"
	sshdriver "github.com/rudderlabs/sql-tunnels/driver/ssh"
)

func TestConnections(t *testing.T) {
	t.Parallel()

	privateKey, err := os.ReadFile("testdata/test_key")
	require.NoError(t, err)
	publicKeyPath, err := filepath.Abs("testdata/test_key.pub")
	require.NoError(t, err)

	pool, err := dockertest.NewPool("")
	require.NoError(t, err)

	// Registered before the resources, so it runs after their containers are purged.
	network, err := pool.CreateNetwork("sql-tunnels-ssh-" + rand.Text())
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := network.Close(); err != nil {
			t.Logf("removing docker network: %v", err)
		}
	})

	pg, err := postgres.Setup(pool, t, postgres.WithNetwork(network.Network))
	require.NoError(t, err)

	ch, err := clickhouse.Setup(pool, t,
		clickhouse.WithNetwork(network.Network),
		clickhouse.WithUser("clickhouse"),
		clickhouse.WithPassword("clickhouse"),
		clickhouse.WithDatabase("db"),
	)
	require.NoError(t, err)

	// Without its own network, sshserver creates one with a fixed name that parallel tests share.
	sshServer, err := sshserver.Setup(pool, t,
		sshserver.WithPublicKeyPath(publicKeyPath),
		sshserver.WithCredentials("linuxserver.io", ""),
		sshserver.WithDockerNetwork(network.Network),
	)
	require.NoError(t, err)

	config := sshdriver.Config{
		User:       "linuxserver.io",
		Host:       sshServer.Host,
		Port:       sshServer.Port,
		PrivateKey: privateKey,
	}

	// The bastion resolves these hostnames on the shared network and dials the container ports.
	postgresDSN := (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(pg.User, pg.Password),
		Host:     net.JoinHostPort(pg.ContainerName, "5432"),
		Path:     "/" + pg.Database,
		RawQuery: "sslmode=disable",
	}).String()
	clickhouseDSN := (&url.URL{
		Scheme: "clickhouse",
		User:   url.UserPassword(ch.User, ch.Password),
		Host:   net.JoinHostPort(ch.Hostname, "9000"),
		Path:   "/" + ch.Database,
	}).String()

	t.Run("postgres", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		encoded, err := config.EncodeWithDSN(postgresDSN)
		require.NoError(t, err)

		db, err := sql.Open("sql+ssh", encoded)
		require.NoError(t, err)

		err = db.PingContext(ctx)
		require.NoError(t, err)

		row := db.QueryRowContext(ctx, "SELECT 1")
		require.NoError(t, row.Err())

		var one int
		err = row.Scan(&one)
		require.NoError(t, err)
		require.Equal(t, 1, one)

		err = db.Close()
		require.NoError(t, err)
	})

	t.Run("clickhouse", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		encoded, err := config.EncodeWithDSN(clickhouseDSN)
		require.NoError(t, err)

		db, err := sql.Open("sql+ssh", encoded)
		require.NoError(t, err)

		err = db.PingContext(ctx)
		require.NoError(t, err)

		row := db.QueryRowContext(ctx, "SELECT 1")
		require.NoError(t, row.Err())

		var one int
		err = row.Scan(&one)
		require.NoError(t, err)
		require.Equal(t, 1, one)

		err = db.Close()
		require.NoError(t, err)
	})
}

package ssh_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	sshdriver "github.com/rudderlabs/sql-tunnels/driver/ssh"
	"github.com/stretchr/testify/require"

	"github.com/rudderlabs/compose-test/testcompose"
)

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

		encoded, err := config.EncodeWithDSN("clickhouse://clickhouse:clickhouse@db_clickhouse:9000/db?skip_verify=false")
		require.NoError(t, err)

		db, err := sql.Open("sql+ssh", encoded)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			return db.PingContext(ctx) == nil
		}, time.Minute, time.Second)

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

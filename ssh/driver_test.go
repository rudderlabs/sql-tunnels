package ssh_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSsh(t *testing.T) {
	db, err := sql.Open("ssh", "postgresql+ssh://11:22@test:12/rudder:password@localhost:7432/jobsdb")
	require.NoError(t, err)
	require.NotNil(t, db)

	err = db.Ping()
	require.EqualError(t, err, "not implemented")
}

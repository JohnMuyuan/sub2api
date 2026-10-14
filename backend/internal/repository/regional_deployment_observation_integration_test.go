//go:build integration

package repository

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// Validate the shipped operational queries against the actual migrated schema.
// This script is aggregate/read-only and must run without privileged writes.
func TestRegionalDeploymentObservationSQL(t *testing.T) {
	raw, err := os.ReadFile("../../../deploy/regional/observe.sql")
	require.NoError(t, err)
	conn, err := integrationDB.Conn(t.Context())
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_, err = conn.ExecContext(t.Context(), string(raw))
	if err != nil {
		_, _ = conn.ExecContext(t.Context(), "ROLLBACK")
	}
	require.NoError(t, err)
}

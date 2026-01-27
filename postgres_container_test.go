package sqltestutil_test

import (
	"os"
	"testing"
	"time"

	"github.com/hostwithquantum/sqltestutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartOptions(t *testing.T) {
	// use bitnami legacy image with pull progress and extended timeout
	pg, err := sqltestutil.StartPostgresContainer(t.Context(), "15.1.0-debian-11-r31", &sqltestutil.StartOptions{
		Image:              "bitnamilegacy/postgresql",
		PullProgressWriter: os.Stdout,
		HealthCheckTimeout: 90 * time.Second,
	})
	require.NoError(t, err)

	defer pg.Shutdown(t.Context())

	assert.NotEmpty(t, pg.ConnectionString())
}

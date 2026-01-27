package sqltestutil_test

import (
	"testing"

	"github.com/hostwithquantum/sqltestutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartOptions(t *testing.T) {
	// use cloudnative-pg's image
	pg, err := sqltestutil.StartPostgresContainer(t.Context(), "15.1.0-debian-11-r31", &sqltestutil.StartOptions{
		Image: "bitnamilegacy/postgresql",
	})
	require.NoError(t, err)

	defer pg.Shutdown(t.Context())

	assert.NotEmpty(t, pg.ConnectionString())
}

package sqltestutil_test

import (
	"testing"

	"github.com/hostwithquantum/sqltestutil"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ContainerTestSuite struct {
	suite.Suite

	pgContainer *sqltestutil.PostgresContainer
}

func (s *ContainerTestSuite) TestContainer() {
	s.T().Run("connection", func(t *testing.T) {
		dsn := s.pgContainer.ConnectionString()
		s.NotEmpty(dsn)
	})

	s.T().Run("ping", func(t *testing.T) {
		conn, err := pgx.Connect(t.Context(), s.pgContainer.ConnectionString())
		s.NoError(err)
		s.NoError(conn.Ping(t.Context()))
	})
}

func TestContainerSuite(t *testing.T) {
	pg, err := sqltestutil.StartPostgresContainer(t.Context(), "14")
	assert.NoError(t, err)
	defer pg.Shutdown(t.Context())

	suite.Run(t, &ContainerTestSuite{
		pgContainer: pg,
	})
}

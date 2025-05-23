package sqltestutil_test

import (
	"context"
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
		conn, err := pgx.Connect(context.TODO(), s.pgContainer.ConnectionString())
		s.NoError(err)
		s.NoError(conn.Ping(context.TODO()))
	})
}

func TestContainerSuite(t *testing.T) {
	ctx := context.Background()

	pg, err := sqltestutil.StartPostgresContainer(context.Background(), "14")
	assert.NoError(t, err)
	defer pg.Shutdown(ctx)

	suite.Run(t, &ContainerTestSuite{
		pgContainer: pg,
	})
}

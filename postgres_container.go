package sqltestutil

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"time"

	"github.com/docker/docker/api/types/container"
	imagetypes "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/jackc/pgx/v5"
)

// PostgresContainer is a Docker container running Postgres. It can be used to
// cheaply start a throwaway Postgres instance for testing.
type PostgresContainer struct {
	id       string
	password string
	port     string
}

// StartOptions configures the behavior of StartPostgresContainer.
type StartOptions struct {
	// HealthCheckTimeout is the maximum time to wait for the container to become healthy.
	// If zero or negative, defaults to 30 seconds.
	HealthCheckTimeout time.Duration

	// Set the base image (e.g. to a private cache/registry)
	Image string

	// PullProgressWriter is an optional writer for image pull progress output.
	// If nil, pull progress is discarded.
	PullProgressWriter io.Writer
}

func (o *StartOptions) healthCheckTimeout() time.Duration {
	if o == nil || o.HealthCheckTimeout <= 0 {
		return 30 * time.Second
	}
	return o.HealthCheckTimeout
}

func (o *StartOptions) image() string {
	if o == nil || o.Image == "" {
		return "postgres"
	}
	return o.Image
}

func (o *StartOptions) pullWriter() io.Writer {
	if o == nil || o.PullProgressWriter == nil {
		return io.Discard
	}
	return o.PullProgressWriter
}

// StartPostgresContainer starts a new Postgres Docker container. The version
// parameter is the tagged version of Postgres image to use, e.g. to use
// postgres:12 pass "12". Creation involes a few steps:
//
// 1. Pull the image if it isn't already cached locally
// 2. Start the container
// 3. Wait for Postgres to be healthy
//
// Once created the container will be immediately usable. It should be stopped
// with the Shutdown method. The container will bind to a randomly available
// host port, and random password. The SQL connection string can be obtained
// with the ConnectionString method.
//
// Container startup and shutdown together can take a few seconds (longer when
// the image has not yet been pulled.) This is generally too slow to initiate in
// each unit test so it's advisable to do setup and teardown once for a whole
// suite of tests. TestMain is one way to do this, however because of Golang
// issue 37206 [1], panics in tests will immediately exit the process without
// giving you the opportunity to Shutdown, which results in orphaned containers
// lying around.
//
// Another approach is to write a single test that starts and stops the
// container and then run sub-tests within there. The testify [2] suite
// package provides a good way to structure these kinds of tests:
//
//	type ExampleTestSuite struct {
//	    suite.Suite
//	}
//
//	func (s *ExampleTestSuite) TestExample() {
//	    // test something
//	}
//
//	func TestExampleTestSuite(t *testing.T) {
//	    pg, _ := sqltestutil.StartPostgresContainer(t.Context(), "12")
//	    defer pg.Shutdown(ctx)
//	    suite.Run(t, &ExampleTestSuite{})
//	}
//
// [1]: https://github.com/golang/go/issues/37206
// [2]: https://github.com/stretchr/testify
func StartPostgresContainer(ctx context.Context, version string, opts ...*StartOptions) (pg *PostgresContainer, err error) {
	var opt *StartOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		panic(err)
	}
	defer cli.Close()

	pg = &PostgresContainer{
		id:       "",
		password: "",
		port:     "",
	}

	image := fmt.Sprintf("%s:%s", opt.image(), version)
	_, err = cli.ImageInspect(ctx, image)
	if err != nil {
		if !client.IsErrNotFound(err) {
			return
		}

		var pullReader io.ReadCloser
		pullReader, err = cli.ImagePull(ctx, image, imagetypes.PullOptions{})
		if err != nil {
			return
		}
		_, err = io.Copy(opt.pullWriter(), pullReader)
		pullReader.Close()
		if err != nil {
			return
		}
	}

	password, err := randomPassword()
	if err != nil {
		return
	}

	pg.password = password

	// Let Docker pick a random port to avoid race conditions
	createResp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: image,
		Env: []string{
			"POSTGRES_DB=pgtest",
			"POSTGRES_PASSWORD=" + pg.password,
			"POSTGRES_USER=pgtest",
		},
		Healthcheck: &container.HealthConfig{
			Test:     []string{"CMD-SHELL", "pg_isready -h 127.0.0.1 -U pgtest -d pgtest"},
			Interval: time.Second,
			Timeout:  5 * time.Second,
			Retries:  30,
		},
	}, &container.HostConfig{
		PortBindings: nat.PortMap{
			"5432/tcp": []nat.PortBinding{
				{HostIP: "127.0.0.1", HostPort: "0"},
			},
		},
	}, nil, nil, "")
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			if removeErr := cli.ContainerRemove(ctx, createResp.ID, container.RemoveOptions{
				Force: true,
			}); removeErr != nil {
				fmt.Println("error removing container:", removeErr)
				return
			}
		}
	}()

	// assign container ID
	pg.id = createResp.ID

	err = cli.ContainerStart(ctx, createResp.ID, container.StartOptions{})
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			stopErr := cli.ContainerStop(ctx, createResp.ID, container.StopOptions{})
			if stopErr != nil {
				fmt.Println("error stopping container:", stopErr)
				return
			}
		}
	}()

	// Wait for container to become healthy with timeout
	timeout := opt.healthCheckTimeout()
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

HealthCheck:
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled while waiting for container: %w", ctx.Err())
		case <-ticker.C:
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("timed out after %v waiting for container to become healthy", timeout)
			}

			inspect, err := cli.ContainerInspect(ctx, createResp.ID)
			if err != nil {
				return nil, err
			}

			// Get the assigned port from the container
			if pg.port == "" && inspect.NetworkSettings != nil {
				if bindings, ok := inspect.NetworkSettings.Ports["5432/tcp"]; ok && len(bindings) > 0 {
					pg.port = bindings[0].HostPort
				}
			}

			if inspect.State.Health == nil {
				// Health check not yet initialized, keep waiting
				continue
			}

			status := inspect.State.Health.Status
			switch status {
			case "unhealthy":
				return nil, errors.New("container unhealthy")
			case "healthy":
				break HealthCheck
			case "starting":
				// Still starting, keep waiting
				continue
			default:
				// Unknown status, keep waiting
				continue
			}
		}
	}

	if pg.port == "" {
		return nil, errors.New("failed to get assigned port from container")
	}

	// Additional verification: ensure database is truly ready to accept queries
	// pg_isready only checks if the server is accepting connections, but the
	// database might still be in startup/recovery mode
	var (
		retryDeadline = time.Now().Add(opt.healthCheckTimeout())
		lastErr       error
		connected     = false
	)

	for time.Now().Before(retryDeadline) {
		conn, connErr := pgx.Connect(ctx, pg.ConnectionString())
		if connErr == nil {
			// Try to execute a simple query
			var result int
			queryErr := conn.QueryRow(ctx, "SELECT 1").Scan(&result)
			conn.Close(ctx)
			if queryErr == nil {
				// Success! Database is ready
				connected = true
				break
			}
			lastErr = queryErr
		} else {
			lastErr = connErr
		}
		// Wait before retrying
		time.Sleep(500 * time.Millisecond)
	}

	if !connected {
		return nil, fmt.Errorf("database not ready after healthcheck passed (timeout: %s): %w", opt.healthCheckTimeout(), lastErr)
	}

	return pg, nil
}

// ConnectionString returns a connection URL string that can be used to connect
// to the running Postgres container.
func (c *PostgresContainer) ConnectionString() string {
	return fmt.Sprintf("postgres://pgtest:%s@127.0.0.1:%s/pgtest", c.password, c.port)
}

// Shutdown cleans up the Postgres container by stopping and removing it. This
// should be called each time a PostgresContainer is created to avoid orphaned
// containers.
func (c *PostgresContainer) Shutdown(ctx context.Context) error {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return err
	}
	defer cli.Close()

	if err := cli.ContainerStop(ctx, c.id, container.StopOptions{}); err != nil {
		return err
	}

	if err := cli.ContainerRemove(ctx, c.id, container.RemoveOptions{}); err != nil {
		return err
	}
	return nil
}

var passwordLetters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randomPassword() (string, error) {
	const passwordLength = 32
	b := make([]rune, passwordLength)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(passwordLetters))))
		if err != nil {
			return "", err
		}
		b[i] = passwordLetters[n.Int64()]
	}
	return string(b), nil
}

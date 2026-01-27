# sqltestutil

[![Documentation](https://godoc.org/github.com/hostwithquantum/sqltestutil?status.svg)](http://godoc.org/github.com/hostwithquantum/sqltestutil)

Utilities for testing Golang code that runs SQL.

> This is a fork, to use: `go get github.com/hostwithquantum/sqltestutil`

## Usage

### PostgresContainer

PostgresContainer is a Docker container running Postgres that can be used to
cheaply start a throwaway Postgres instance for testing.

```go
// Default 30 second timeout
pg, err := sqltestutil.StartPostgresContainer(ctx, "14")

// Custom timeout
pg, err := sqltestutil.StartPostgresContainer(ctx, "14", &sqltestutil.StartOption{
    HealthCheckTimeout: 60 * time.Second,
})
```

### RunMigration

RunMigration reads all of the files matching *.up.sql in a directory and
executes them in lexicographical order against the provided DB.

### LoadScenario

LoadScenario reads a YAML "scenario" file and uses it to populate the given DB.

### Suite

Suite is a [testify
suite](https://pkg.go.dev/github.com/stretchr/testify@v1.7.0/suite#Suite) that
provides a database connection for running tests against a SQL database.

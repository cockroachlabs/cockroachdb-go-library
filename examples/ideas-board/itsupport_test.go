package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/cockroachdb"
)

// startContainer boots a throwaway CockroachDB container on the given image and
// returns its connection string. It skips the test cleanly when Docker is
// unavailable, so the integration tests become no-ops (SKIP) rather than failures
// in environments without a container runtime.
func startContainer(t *testing.T, image string) (connString string, cleanup func()) {
	t.Helper()
	t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	ctx := context.Background()

	container, err := cockroachdb.Run(ctx, image,
		cockroachdb.WithInsecure(),
		cockroachdb.WithDatabase("ideasdb"),
	)
	if err != nil {
		t.Skipf("skipping integration test: cannot start CockroachDB container (Docker required): %v", err)
	}

	// The testcontainers cockroachdb module v0.44 ConnectionString returns a pgx
	// stdlib registered-config KEY, not a URL, so build the DSN from host + mapped
	// port (same workaround as service_concurrency_test.go).
	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "26257/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("mapped port: %v", err)
	}
	cs := fmt.Sprintf("postgres://root@%s:%s/ideasdb?sslmode=disable", host, port.Port())

	return cs, func() { _ = container.Terminate(context.Background()) }
}

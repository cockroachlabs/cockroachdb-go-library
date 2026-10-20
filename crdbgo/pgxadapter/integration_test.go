//go:build integration

package pgxadapter_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/pgxadapter"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/testutil"
	"github.com/jackc/pgx/v5"
)

func TestIntegration_NewPool(t *testing.T) {
	cs := testutil.ConnString(t)
	ctx := context.Background()

	pool, err := pgxadapter.NewPool(ctx, cs, nil)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	defer pool.Close()
}

func TestIntegration_Ping(t *testing.T) {
	cs := testutil.ConnString(t)
	ctx := context.Background()

	pool, err := pgxadapter.NewPool(ctx, cs, nil)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestIntegration_Check(t *testing.T) {
	cs := testutil.ConnString(t)
	ctx := context.Background()

	pool, err := pgxadapter.NewPool(ctx, cs, nil)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	defer pool.Close()

	if err := pool.Check(ctx); err != nil {
		t.Fatalf("Check (health) failed: %v", err)
	}
}

func TestIntegration_ExecuteTx_Commit(t *testing.T) {
	cs := testutil.ConnString(t)
	ctx := context.Background()
	table := testutil.UniqueTable("pgx_tx")

	pool, err := pgxadapter.NewPool(ctx, cs, nil)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	defer pool.Close()

	// Create table and insert inside a transaction.
	err = pool.ExecuteTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s (id INT PRIMARY KEY, name TEXT)", table)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			"INSERT INTO %s (id, name) VALUES (1, 'alice')", table)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteTx (create+insert) failed: %v", err)
	}

	// Verify data was committed by reading it back.
	var name string
	err = pool.ExecuteTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, fmt.Sprintf("SELECT name FROM %s WHERE id = 1", table)).Scan(&name)
	})
	if err != nil {
		t.Fatalf("ExecuteTx (select) failed: %v", err)
	}
	if name != "alice" {
		t.Errorf("expected name 'alice', got %q", name)
	}
}

func TestIntegration_ExecuteTx_Retry(t *testing.T) {
	cs := testutil.ConnString(t)
	ctx := context.Background()
	table := testutil.UniqueTable("pgx_retry")

	// Create pool with retry enabled.
	pool, err := pgxadapter.NewPool(ctx, cs, []crdbgo.Option{
		crdbgo.WithMaxRetries(5),
	})
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	defer pool.Close()

	// Set up a table with a single row.
	err = pool.ExecuteTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s (id INT PRIMARY KEY, counter INT DEFAULT 0)", table)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, fmt.Sprintf("INSERT INTO %s (id, counter) VALUES (1, 0)", table))
		return err
	})
	if err != nil {
		t.Fatalf("setup table failed: %v", err)
	}

	// Run concurrent updates to trigger serialization conflicts.
	const goroutines = 5
	var wg sync.WaitGroup
	errors := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errors[idx] = pool.ExecuteTx(ctx, func(tx pgx.Tx) error {
				var counter int
				if err := tx.QueryRow(ctx, fmt.Sprintf(
					"SELECT counter FROM %s WHERE id = 1", table)).Scan(&counter); err != nil {
					return err
				}
				_, err := tx.Exec(ctx, fmt.Sprintf(
					"UPDATE %s SET counter = $1 WHERE id = 1", table), counter+1)
				return err
			})
		}(i)
	}
	wg.Wait()

	// At least some should succeed thanks to retry logic.
	var successes int
	for _, err := range errors {
		if err == nil {
			successes++
		}
	}
	if successes == 0 {
		t.Error("expected at least one concurrent transaction to succeed with retries")
	}

	// Verify the final counter value is positive.
	var finalCounter int
	err = pool.ExecuteTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, fmt.Sprintf("SELECT counter FROM %s WHERE id = 1", table)).Scan(&finalCounter)
	})
	if err != nil {
		t.Fatalf("read final counter: %v", err)
	}
	if finalCounter <= 0 {
		t.Errorf("expected positive counter, got %d", finalCounter)
	}
}

func TestIntegration_FollowerRead(t *testing.T) {
	cs := testutil.ConnString(t)
	ctx := context.Background()
	table := testutil.UniqueTable("pgx_follower")

	pool, err := pgxadapter.NewPool(ctx, cs, nil)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	defer pool.Close()

	// Create table with data.
	err = pool.ExecuteTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s (id INT PRIMARY KEY, val TEXT)", table)); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, fmt.Sprintf(
			"INSERT INTO %s (id, val) VALUES (1, 'hello')", table))
		return err
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Test QueryFollowerRead — should succeed on single-node CockroachDB.
	rows, err := pool.QueryFollowerRead(ctx, fmt.Sprintf("SELECT val FROM %s WHERE id = 1", table))
	if err != nil {
		t.Fatalf("QueryFollowerRead failed: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("QueryFollowerRead returned no rows")
	}
	var val string
	if err := rows.Scan(&val); err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if val != "hello" {
		t.Errorf("expected 'hello', got %q", val)
	}

	// Test QueryRowFollowerRead.
	var val2 string
	err = pool.QueryRowFollowerRead(ctx, fmt.Sprintf("SELECT val FROM %s WHERE id = 1", table)).Scan(&val2)
	if err != nil {
		t.Fatalf("QueryRowFollowerRead failed: %v", err)
	}
	if val2 != "hello" {
		t.Errorf("expected 'hello', got %q", val2)
	}
}

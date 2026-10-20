//go:build integration

package sqlxadapter_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo/sqlxadapter"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/testutil"
	"github.com/jmoiron/sqlx"
)

func TestIntegration_Open(t *testing.T) {
	cs := testutil.ConnString(t)

	db, err := sqlxadapter.Open(cs, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if err := db.Ping(context.Background()); err != nil {
		t.Fatalf("Ping after Open failed: %v", err)
	}
}

func TestIntegration_Check(t *testing.T) {
	cs := testutil.ConnString(t)

	db, err := sqlxadapter.Open(cs, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if err := db.Check(context.Background()); err != nil {
		t.Fatalf("Check (health) failed: %v", err)
	}
}

func TestIntegration_ExecuteTx_Commit(t *testing.T) {
	cs := testutil.ConnString(t)
	ctx := context.Background()
	table := testutil.UniqueTable("sqlx_tx")

	db, err := sqlxadapter.Open(cs, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Create table and insert a row inside a transaction.
	err = db.ExecuteTx(ctx, func(tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s (id INT PRIMARY KEY, name TEXT)", table)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, fmt.Sprintf(
			"INSERT INTO %s (id, name) VALUES (1, 'bob')", table))
		return err
	})
	if err != nil {
		t.Fatalf("ExecuteTx (create+insert) failed: %v", err)
	}

	// Verify committed data.
	var name string
	err = db.ExecuteTx(ctx, func(tx *sqlx.Tx) error {
		return tx.QueryRowContext(ctx, fmt.Sprintf(
			"SELECT name FROM %s WHERE id = 1", table)).Scan(&name)
	})
	if err != nil {
		t.Fatalf("ExecuteTx (select) failed: %v", err)
	}
	if name != "bob" {
		t.Errorf("expected 'bob', got %q", name)
	}
}

func TestIntegration_QueryFollowerRead(t *testing.T) {
	cs := testutil.ConnString(t)
	ctx := context.Background()
	table := testutil.UniqueTable("sqlx_follower")

	db, err := sqlxadapter.Open(cs, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Setup table with data.
	err = db.ExecuteTx(ctx, func(tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s (id INT PRIMARY KEY, val TEXT)", table)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, fmt.Sprintf(
			"INSERT INTO %s (id, val) VALUES (1, 'world')", table))
		return err
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Test QueryFollowerRead.
	rows, err := db.QueryFollowerRead(ctx, fmt.Sprintf("SELECT val FROM %s WHERE id = 1", table))
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
	if val != "world" {
		t.Errorf("expected 'world', got %q", val)
	}
}

func TestIntegration_GetFollowerRead(t *testing.T) {
	cs := testutil.ConnString(t)
	ctx := context.Background()
	table := testutil.UniqueTable("sqlx_get_fr")

	db, err := sqlxadapter.Open(cs, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Setup.
	err = db.ExecuteTx(ctx, func(tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s (id INT PRIMARY KEY, val TEXT)", table)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, fmt.Sprintf(
			"INSERT INTO %s (id, val) VALUES (1, 'fr_get')", table))
		return err
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	type row struct {
		ID  int    `db:"id"`
		Val string `db:"val"`
	}

	var r row
	err = db.GetFollowerRead(ctx, &r, fmt.Sprintf("SELECT id, val FROM %s WHERE id = 1", table))
	if err != nil {
		t.Fatalf("GetFollowerRead failed: %v", err)
	}
	if r.Val != "fr_get" {
		t.Errorf("expected 'fr_get', got %q", r.Val)
	}
}

func TestIntegration_SelectFollowerRead(t *testing.T) {
	cs := testutil.ConnString(t)
	ctx := context.Background()
	table := testutil.UniqueTable("sqlx_sel_fr")

	db, err := sqlxadapter.Open(cs, nil)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Setup with multiple rows.
	err = db.ExecuteTx(ctx, func(tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s (id INT PRIMARY KEY, val TEXT)", table)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(
			"INSERT INTO %s (id, val) VALUES (1, 'a'), (2, 'b'), (3, 'c')", table)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	type row struct {
		ID  int    `db:"id"`
		Val string `db:"val"`
	}

	var results []row
	err = db.SelectFollowerRead(ctx, &results, fmt.Sprintf("SELECT id, val FROM %s ORDER BY id", table))
	if err != nil {
		t.Fatalf("SelectFollowerRead failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Val != "a" || results[1].Val != "b" || results[2].Val != "c" {
		t.Errorf("unexpected values: %+v", results)
	}
}

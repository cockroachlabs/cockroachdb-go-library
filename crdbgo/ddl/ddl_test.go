package ddl_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo/ddl"
	"github.com/jackc/pgx/v5"
)

func crdbConn(t *testing.T) *pgx.Conn {
	t.Helper()
	url := os.Getenv("CRDB_URL")
	if url == "" {
		url = "postgresql://root@localhost:26257/defaultdb?sslmode=disable"
	}
	conn, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect to CockroachDB: %v", err)
	}
	return conn
}

func uniqueTable(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, os.Getpid())
}

func getIndexNames(t *testing.T, conn *pgx.Conn, table string) map[string]bool {
	t.Helper()
	rows, err := conn.Query(context.Background(), "SHOW INDEXES FROM "+table)
	if err != nil {
		t.Fatalf("SHOW INDEXES: %v", err)
	}
	defer rows.Close()
	names := map[string]bool{}
	for rows.Next() {
		vals, _ := rows.Values()
		if len(vals) > 1 {
			names[fmt.Sprint(vals[1])] = true
		}
	}
	return names
}

func TestCreateTableWithIndexes(t *testing.T) {
	conn := crdbConn(t)
	ctx := context.Background()
	defer conn.Close(ctx)

	table := uniqueTable("t_idx")
	sql := ddl.CreateTableWithIndexes(table, []ddl.ColumnDef{
		{Name: "id", Type: "UUID", PrimaryKey: true, Default: "gen_random_uuid()"},
		{Name: "customer", Type: "STRING(100)", NotNull: true},
		{Name: "total", Type: "INT8"},
	}, []ddl.IndexDef{
		{Name: "idx_cust", Columns: []string{"customer"}},
		{Name: "idx_tot", Columns: []string{"total"}, Storing: []string{"customer"}},
	})

	if !strings.Contains(sql, "INDEX idx_cust (customer)") {
		t.Errorf("DDL missing inline index, got: %s", sql)
	}
	if !strings.Contains(sql, "STORING (customer)") {
		t.Errorf("DDL missing STORING clause, got: %s", sql)
	}

	_, err := conn.Exec(ctx, sql)
	if err != nil {
		t.Fatalf("CREATE TABLE failed on live CRDB: %v\nSQL: %s", err, sql)
	}
	defer conn.Exec(ctx, "DROP TABLE IF EXISTS "+table)

	indexes := getIndexNames(t, conn, table)
	if !indexes["idx_cust"] {
		t.Errorf("idx_cust not found in SHOW INDEXES: %v", indexes)
	}
	if !indexes["idx_tot"] {
		t.Errorf("idx_tot not found in SHOW INDEXES: %v", indexes)
	}

	_, err = conn.Exec(ctx, fmt.Sprintf("INSERT INTO %s (customer, total) VALUES ('Alice', 100)", table))
	if err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}
	var cust string
	err = conn.QueryRow(ctx, fmt.Sprintf("SELECT customer FROM %s WHERE total = 100", table)).Scan(&cust)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if cust != "Alice" {
		t.Errorf("expected 'Alice', got %q", cust)
	}
}

func TestCreateTableWithUniqueIndex(t *testing.T) {
	conn := crdbConn(t)
	ctx := context.Background()
	defer conn.Close(ctx)

	table := uniqueTable("t_uniq")
	sql := ddl.CreateTableWithIndexes(table, []ddl.ColumnDef{
		{Name: "id", Type: "UUID", PrimaryKey: true, Default: "gen_random_uuid()"},
		{Name: "email", Type: "STRING(255)", NotNull: true},
	}, []ddl.IndexDef{
		{Name: "idx_email", Columns: []string{"email"}, Unique: true},
	})

	if !strings.Contains(sql, "UNIQUE INDEX idx_email") {
		t.Errorf("DDL missing UNIQUE keyword, got: %s", sql)
	}

	_, err := conn.Exec(ctx, sql)
	if err != nil {
		t.Fatalf("CREATE TABLE failed: %v\nSQL: %s", err, sql)
	}
	defer conn.Exec(ctx, "DROP TABLE IF EXISTS "+table)

	_, err = conn.Exec(ctx, fmt.Sprintf("INSERT INTO %s (email) VALUES ('a@b.com')", table))
	if err != nil {
		t.Fatalf("first INSERT failed: %v", err)
	}
	_, err = conn.Exec(ctx, fmt.Sprintf("INSERT INTO %s (email) VALUES ('a@b.com')", table))
	if err == nil {
		t.Error("expected unique violation on duplicate email, got nil")
	} else if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected duplicate key error, got: %v", err)
	}
}

func TestCreateTableNoIndexes(t *testing.T) {
	conn := crdbConn(t)
	ctx := context.Background()
	defer conn.Close(ctx)

	table := uniqueTable("t_noidx")
	sql := ddl.CreateTableWithIndexes(table, []ddl.ColumnDef{
		{Name: "id", Type: "INT8", PrimaryKey: true, Default: "unique_rowid()"},
		{Name: "name", Type: "STRING(100)"},
	}, nil)

	_, err := conn.Exec(ctx, sql)
	if err != nil {
		t.Fatalf("CREATE TABLE failed: %v\nSQL: %s", err, sql)
	}
	defer conn.Exec(ctx, "DROP TABLE IF EXISTS "+table)

	_, err = conn.Exec(ctx, fmt.Sprintf("INSERT INTO %s (name) VALUES ('test')", table))
	if err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}
	var name string
	err = conn.QueryRow(ctx, fmt.Sprintf("SELECT name FROM %s LIMIT 1", table)).Scan(&name)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if name != "test" {
		t.Errorf("expected 'test', got %q", name)
	}
}

func TestCreateTableWithDefault(t *testing.T) {
	conn := crdbConn(t)
	ctx := context.Background()
	defer conn.Close(ctx)

	table := uniqueTable("t_default")
	sql := ddl.CreateTableWithIndexes(table, []ddl.ColumnDef{
		{Name: "id", Type: "UUID", PrimaryKey: true, Default: "gen_random_uuid()"},
		{Name: "val", Type: "INT8", Default: "42"},
	}, nil)

	if !strings.Contains(sql, "DEFAULT gen_random_uuid()") {
		t.Errorf("DDL missing DEFAULT, got: %s", sql)
	}

	_, err := conn.Exec(ctx, sql)
	if err != nil {
		t.Fatalf("CREATE TABLE failed: %v\nSQL: %s", err, sql)
	}
	defer conn.Exec(ctx, "DROP TABLE IF EXISTS "+table)

	_, err = conn.Exec(ctx, fmt.Sprintf("INSERT INTO %s DEFAULT VALUES", table))
	if err != nil {
		t.Fatalf("INSERT DEFAULT VALUES failed: %v", err)
	}
	var val int
	err = conn.QueryRow(ctx, fmt.Sprintf("SELECT val FROM %s LIMIT 1", table)).Scan(&val)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if val != 42 {
		t.Errorf("expected default 42, got %d", val)
	}
}

func TestComputedColumnDDL(t *testing.T) {
	conn := crdbConn(t)
	ctx := context.Background()
	defer conn.Close(ctx)

	table := uniqueTable("t_computed")
	_, err := conn.Exec(ctx, fmt.Sprintf(
		"CREATE TABLE %s (id INT8 PRIMARY KEY DEFAULT unique_rowid(), price INT8, qty INT8)", table))
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	defer conn.Exec(ctx, "DROP TABLE IF EXISTS "+table)

	alterSQL := ddl.ComputedColumnDDL(table, ddl.ComputedColumnDef{
		Name:       "total",
		Type:       "INT8",
		Expression: "price * qty",
		NotNull:    true,
	})

	if !strings.Contains(alterSQL, "AS (price * qty) STORED") {
		t.Errorf("DDL missing computed expression, got: %s", alterSQL)
	}

	_, err = conn.Exec(ctx, alterSQL)
	if err != nil {
		t.Fatalf("ALTER TABLE failed: %v\nSQL: %s", err, alterSQL)
	}

	_, err = conn.Exec(ctx, fmt.Sprintf("INSERT INTO %s (price, qty) VALUES (10, 5)", table))
	if err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	var total int
	err = conn.QueryRow(ctx, fmt.Sprintf("SELECT total FROM %s LIMIT 1", table)).Scan(&total)
	if err != nil {
		t.Fatalf("SELECT total: %v", err)
	}
	if total != 50 {
		t.Errorf("expected total=50, got %d", total)
	}
}

func TestComputedColumnDDLDefaultType(t *testing.T) {
	conn := crdbConn(t)
	ctx := context.Background()
	defer conn.Close(ctx)

	table := uniqueTable("t_comp_def")
	_, err := conn.Exec(ctx, fmt.Sprintf(
		"CREATE TABLE %s (id INT8 PRIMARY KEY DEFAULT unique_rowid(), a INT8, b INT8)", table))
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	defer conn.Exec(ctx, "DROP TABLE IF EXISTS "+table)

	alterSQL := ddl.ComputedColumnDDL(table, ddl.ComputedColumnDef{
		Name:       "sum_col",
		Expression: "a + b",
	})

	if !strings.Contains(alterSQL, "INT8") {
		t.Errorf("should default to INT8, got: %s", alterSQL)
	}

	_, err = conn.Exec(ctx, alterSQL)
	if err != nil {
		t.Fatalf("ALTER TABLE failed: %v\nSQL: %s", err, alterSQL)
	}

	_, err = conn.Exec(ctx, fmt.Sprintf("INSERT INTO %s (a, b) VALUES (3, 7)", table))
	if err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	var sum int
	err = conn.QueryRow(ctx, fmt.Sprintf("SELECT sum_col FROM %s LIMIT 1", table)).Scan(&sum)
	if err != nil {
		t.Fatalf("SELECT sum_col: %v", err)
	}
	if sum != 10 {
		t.Errorf("expected sum_col=10, got %d", sum)
	}
}

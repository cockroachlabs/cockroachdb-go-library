package gormadapter

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// regionalRow is a REGIONAL BY ROW model mapping the hidden crdb_region column.
type regionalRow struct {
	ID         string `gorm:"column:id;primaryKey"`
	Text       string `gorm:"column:text"`
	CrdbRegion string `gorm:"column:crdb_region"`
}

func (regionalRow) TableName() string { return "regional_idea" }

// newDryRunDB opens a GORM DB that builds SQL without ever connecting:
// sql.Open is lazy and DryRun skips execution, so no CockroachDB is required.
func newDryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	sqlDB, err := sql.Open("pgx", "postgres://u:p@127.0.0.1:5432/db?sslmode=disable")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	if err := gdb.Use(NewPlugin(nil)); err != nil {
		t.Fatalf("Use plugin: %v", err)
	}
	return gdb
}

func containsVar(vars []any, want string) bool {
	for _, v := range vars {
		if s, ok := v.(string); ok && s == want {
			return true
		}
	}
	return false
}

// TestInRegion_StampsCrdbRegionColumn proves the InRegion scope homes a created
// row by writing the crdb_region column on INSERT, never via SET crdb_region
// (which CockroachDB rejects — there is no such session variable).
//
// Teeth: against the old SET-based code the INSERT carries no crdb_region
// column, so the first assertion fails.
func TestInRegion_StampsCrdbRegionColumn(t *testing.T) {
	db := newDryRunDB(t)

	tx := db.Session(&gorm.Session{DryRun: true}).
		Scopes(InRegion("us-west1")).
		Create(&regionalRow{ID: "idea-west", Text: "home me west"})
	if tx.Error != nil {
		t.Fatalf("create: %v", tx.Error)
	}

	sqlStr := tx.Statement.SQL.String()
	if !strings.Contains(sqlStr, "crdb_region") {
		t.Fatalf("INSERT did not stamp crdb_region column: %s", sqlStr)
	}
	if strings.Contains(strings.ToUpper(sqlStr), "SET CRDB_REGION") {
		t.Fatalf("must not use the invalid SET crdb_region session var: %s", sqlStr)
	}
	if !containsVar(tx.Statement.Vars, "us-west1") {
		t.Fatalf("region us-west1 not bound; vars=%v sql=%s", tx.Statement.Vars, sqlStr)
	}
}

// TestContextRegion_StampsCrdbRegionColumn proves a RegionContext on the
// statement context homes the created row via the crdb_region column.
func TestContextRegion_StampsCrdbRegionColumn(t *testing.T) {
	db := newDryRunDB(t)
	ctx := crdbgo.WithRegionContext(context.Background(), &crdbgo.RegionContext{Region: "us-east1"})

	tx := db.Session(&gorm.Session{DryRun: true}).
		WithContext(ctx).
		Create(&regionalRow{ID: "idea-east", Text: "home me east"})
	if tx.Error != nil {
		t.Fatalf("create: %v", tx.Error)
	}

	sqlStr := tx.Statement.SQL.String()
	if !strings.Contains(sqlStr, "crdb_region") {
		t.Fatalf("INSERT did not stamp crdb_region from context: %s", sqlStr)
	}
	if !containsVar(tx.Statement.Vars, "us-east1") {
		t.Fatalf("region us-east1 not bound; vars=%v sql=%s", tx.Statement.Vars, sqlStr)
	}
}

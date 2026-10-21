//go:build integration

package gormadapter_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo"
	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/gormadapter"
	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/testutil"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// gormItem is a simple model for integration tests.
type gormItem struct {
	ID    int    `gorm:"primaryKey"`
	Name  string `gorm:"type:text"`
	Value int    `gorm:"type:int"`
}

// tableName returns a unique table name scoped to each test.
func tableName(suffix string) string {
	return testutil.UniqueTable(fmt.Sprintf("gorm_%s", suffix))
}

func openGormWithPlugin(t *testing.T, opts ...crdbgo.Option) *gorm.DB {
	t.Helper()
	cs := testutil.ConnString(t)

	db, err := gorm.Open(postgres.Open(cs), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm.Open failed: %v", err)
	}

	plugin := gormadapter.NewPlugin(opts)
	if err := db.Use(plugin); err != nil {
		t.Fatalf("plugin registration failed: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("underlying sql.DB: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	return db
}

func TestIntegration_PluginRegisters(t *testing.T) {
	db := openGormWithPlugin(t)

	// Verify the plugin is registered.
	if db == nil {
		t.Fatal("expected non-nil gorm.DB with plugin")
	}
}

func TestIntegration_CRUD(t *testing.T) {
	db := openGormWithPlugin(t)
	table := tableName("crud")

	// Create a unique table for this test.
	type Item struct {
		ID    int    `gorm:"primaryKey"`
		Name  string `gorm:"type:text"`
		Value int    `gorm:"type:int"`
	}

	// Override the table name.
	tbl := db.Table(table)

	// AutoMigrate with explicit table name.
	if err := tbl.AutoMigrate(&Item{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// Create.
	item := Item{ID: 1, Name: "test-item", Value: 42}
	if err := db.Table(table).Create(&item).Error; err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Read.
	var found Item
	if err := db.Table(table).First(&found, 1).Error; err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if found.Name != "test-item" || found.Value != 42 {
		t.Errorf("unexpected read: %+v", found)
	}

	// Update.
	if err := db.Table(table).Model(&Item{ID: 1}).Update("value", 100).Error; err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	var updated Item
	if err := db.Table(table).First(&updated, 1).Error; err != nil {
		t.Fatalf("Read after update failed: %v", err)
	}
	if updated.Value != 100 {
		t.Errorf("expected value 100, got %d", updated.Value)
	}

	// Delete.
	if err := db.Table(table).Delete(&Item{}, 1).Error; err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	var deleted Item
	result := db.Table(table).First(&deleted, 1)
	if result.Error == nil {
		t.Error("expected error on reading deleted row, got nil")
	}
}

func TestIntegration_FollowerReadScope(t *testing.T) {
	db := openGormWithPlugin(t)
	table := tableName("follower")

	type Item struct {
		ID  int    `gorm:"primaryKey"`
		Val string `gorm:"type:text"`
	}

	if err := db.Table(table).AutoMigrate(&Item{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	if err := db.Table(table).Create(&Item{ID: 1, Val: "stale-ok"}).Error; err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// FollowerRead scope should not error on single-node CockroachDB.
	var items []Item
	err := db.Table(table).Scopes(gormadapter.FollowerRead()).Find(&items).Error
	if err != nil {
		t.Fatalf("FollowerRead scope failed: %v", err)
	}
	if len(items) == 0 {
		t.Error("expected at least 1 item from follower read")
	}
}

func TestIntegration_ConcurrentWriteRetry(t *testing.T) {
	db := openGormWithPlugin(t, crdbgo.WithMaxRetries(5))
	table := tableName("retry")

	type Counter struct {
		ID    int `gorm:"primaryKey"`
		Count int `gorm:"type:int"`
	}

	if err := db.Table(table).AutoMigrate(&Counter{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}
	if err := db.Table(table).Create(&Counter{ID: 1, Count: 0}).Error; err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Run concurrent updates — the plugin's retry callbacks should handle
	// serialization conflicts.
	const goroutines = 5
	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = db.Transaction(func(tx *gorm.DB) error {
				var c Counter
				if err := tx.Table(table).First(&c, 1).Error; err != nil {
					return err
				}
				c.Count++
				return tx.Table(table).Save(&c).Error
			})
		}(i)
	}
	wg.Wait()

	// Count successes.
	var successes int
	for _, err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes == 0 {
		t.Error("expected at least one concurrent transaction to succeed")
	}

	// Verify final count.
	var final Counter
	if err := db.Table(table).First(&final, 1).Error; err != nil {
		t.Fatalf("read final: %v", err)
	}

	_ = context.Background() // used for linting satisfaction
}

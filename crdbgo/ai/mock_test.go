package ai

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
)

// mockDB implements SQLDB for unit testing without a real database.
type mockDB struct {
	mu          sync.Mutex
	queryFn     func(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	execFn      func(ctx context.Context, query string, args ...any) (sql.Result, error)
	beginTxFn   func(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
	execQueries []string
}

func newMockDB() *mockDB {
	return &mockDB{}
}

func (m *mockDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	m.mu.Lock()
	m.execQueries = append(m.execQueries, query)
	m.mu.Unlock()
	if m.queryFn != nil {
		return m.queryFn(ctx, query, args...)
	}
	return nil, errors.New("mock: QueryContext not configured")
}

func (m *mockDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	m.mu.Lock()
	m.execQueries = append(m.execQueries, query)
	m.mu.Unlock()
	if m.execFn != nil {
		return m.execFn(ctx, query, args...)
	}
	return mockResult{}, nil
}

func (m *mockDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	if m.beginTxFn != nil {
		return m.beginTxFn(ctx, opts)
	}
	return nil, errors.New("mock: BeginTx not configured")
}

func (m *mockDB) queries() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.execQueries))
	copy(cp, m.execQueries)
	return cp
}

type mockResult struct{}

func (mockResult) LastInsertId() (int64, error) { return 0, nil }
func (mockResult) RowsAffected() (int64, error) { return 1, nil }

// mockEmbedder implements EmbeddingGenerator for testing.
type mockEmbedder struct {
	dims      int
	embedding []float32
	err       error
	calls     int
	mu        sync.Mutex
}

func newMockEmbedder(dims int) *mockEmbedder {
	emb := make([]float32, dims)
	for i := range emb {
		emb[i] = float32(i) * 0.01
	}
	return &mockEmbedder{dims: dims, embedding: emb}
}

func (m *mockEmbedder) GenerateEmbedding(_ context.Context, _ string) ([]float32, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	cp := make([]float32, len(m.embedding))
	copy(cp, m.embedding)
	return cp, nil
}

func (m *mockEmbedder) Dimensions() int { return m.dims }

func (m *mockEmbedder) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// mockChatProvider implements ChatCompletionProvider for testing.
type mockChatProvider struct {
	response string
	err      error
	messages []ChatMessage
}

func (m *mockChatProvider) Complete(_ context.Context, messages []ChatMessage, _ float64, _ int) (string, error) {
	m.messages = messages
	return m.response, m.err
}

func (m *mockChatProvider) Stream(_ context.Context, messages []ChatMessage, _ float64, _ int) (<-chan string, <-chan error) {
	m.messages = messages
	tokenCh := make(chan string, 10)
	errCh := make(chan error, 1)
	go func() {
		defer close(tokenCh)
		if m.err != nil {
			errCh <- m.err
			return
		}
		for _, word := range []string{"Hello", " ", "world"} {
			tokenCh <- word
		}
		errCh <- nil
	}()
	return tokenCh, errCh
}

// fakeDriver registers a test database/sql driver for transaction testing.
var (
	fakeDriverOnce sync.Once
	fakeDriverDB   *sql.DB
)

func getFakeDB(t interface{ Helper(); Cleanup(func()) }) *sql.DB {
	t.Helper()
	fakeDriverOnce.Do(func() {
		sql.Register("fakeai", &fakeAIDriver{})
		db, err := sql.Open("fakeai", "")
		if err != nil {
			panic(err)
		}
		fakeDriverDB = db
	})
	return fakeDriverDB
}

type fakeAIDriver struct{}

func (fakeAIDriver) Open(_ string) (driver.Conn, error) {
	return &fakeConn{}, nil
}

type fakeConn struct{}

func (fakeConn) Prepare(query string) (driver.Stmt, error) {
	return &fakeStmt{query: query}, nil
}

func (fakeConn) Close() error { return nil }

func (fakeConn) Begin() (driver.Tx, error) {
	return &fakeTx{}, nil
}

func (fakeConn) BeginTx(_ context.Context, _ driver.TxOptions) (driver.Tx, error) {
	return &fakeTx{}, nil
}

var _ driver.ConnBeginTx = fakeConn{}

type fakeStmt struct {
	query string
}

func (fakeStmt) Close() error                                    { return nil }
func (fakeStmt) NumInput() int                                   { return -1 }
func (fakeStmt) Exec(args []driver.Value) (driver.Result, error) { return fakeDriverResult{}, nil }
func (s fakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	if strings.Contains(s.query, "content") || strings.Contains(s.query, "metadata") {
		return newStoreRows(), nil
	}
	return newSearchRows(), nil
}

type fakeDriverResult struct{}

func (fakeDriverResult) LastInsertId() (int64, error) { return 0, nil }
func (fakeDriverResult) RowsAffected() (int64, error) { return 1, nil }

type fakeRows struct {
	done    bool
	columns []string
	values  []driver.Value
}

func newSearchRows() *fakeRows {
	return &fakeRows{
		columns: []string{"id", "distance"},
		values:  []driver.Value{"doc-1", 0.15},
	}
}

func newStoreRows() *fakeRows {
	return &fakeRows{
		columns: []string{"id", "content", "metadata", "distance"},
		values:  []driver.Value{"doc-1", "test content", "{}", 0.15},
	}
}

func (r *fakeRows) Columns() []string { return r.columns }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	copy(dest, r.values)
	return nil
}

type fakeTx struct{}

func (fakeTx) Commit() error   { return nil }
func (fakeTx) Rollback() error { return nil }

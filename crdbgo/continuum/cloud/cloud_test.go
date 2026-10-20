package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestNew_SelectsSimulatedWhenNoKey(t *testing.T) {
	if _, ok := New(Config{}).(*SimulatedClient); !ok {
		t.Error("New(zero Config) should return *SimulatedClient")
	}
	if _, ok := New(Config{Enabled: true}).(*SimulatedClient); !ok {
		t.Error("New enabled but no API key should return *SimulatedClient")
	}
}

func TestNew_SelectsHTTPWhenKeySet(t *testing.T) {
	if _, ok := New(Config{Enabled: true, APIKey: "sk"}).(*HTTPClient); !ok {
		t.Error("New with API key should return *HTTPClient")
	}
}

func TestSimulated_ClustersAndMetricExport(t *testing.T) {
	c := NewSimulated()
	cls, err := c.ListClusters(context.Background())
	if err != nil || len(cls) != 1 {
		t.Fatalf("ListClusters: got %d clusters, err=%v", len(cls), err)
	}
	got, err := c.GetCluster(context.Background(), cls[0].ID)
	if err != nil || got.Name != "dev-cluster" {
		t.Fatalf("GetCluster: got %+v, err=%v", got, err)
	}
	if _, err := c.GetCluster(context.Background(), "missing"); err == nil {
		t.Error("GetCluster(missing) should error")
	}
	if err := c.ConfigureMetricExport(context.Background(), cls[0].ID,
		MetricExportConfig{Target: MetricExportDatadog, Settings: map[string]any{"site": "us5", "api_key": "dd"}}); err != nil {
		t.Fatalf("ConfigureMetricExport: %v", err)
	}
	if ex := c.MetricExports(); ex[cls[0].ID].Target != MetricExportDatadog {
		t.Errorf("metric export not recorded: %+v", ex)
	}
	if err := c.ConfigureMetricExport(context.Background(), "x", MetricExportConfig{}); err == nil {
		t.Error("ConfigureMetricExport with no target should error")
	}
}

func TestHTTP_ListGetAndAuth(t *testing.T) {
	var sawAuth, sawPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		sawPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/clusters":
			json.NewEncoder(w).Encode(map[string]any{"clusters": []Cluster{{ID: "c1", Name: "prod", Plan: "STANDARD"}}})
		case "/clusters/c1":
			json.NewEncoder(w).Encode(Cluster{ID: "c1", Name: "prod", Plan: "STANDARD"})
		default:
			http.Error(w, "nope", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewHTTP(Config{Enabled: true, APIKey: "sk-123", BaseURL: srv.URL, HTTP: srv.Client()})
	cls, err := c.ListClusters(context.Background())
	if err != nil || len(cls) != 1 || cls[0].ID != "c1" {
		t.Fatalf("ListClusters: %+v err=%v", cls, err)
	}
	if sawAuth != "Bearer sk-123" {
		t.Errorf("Authorization header = %q, want Bearer sk-123", sawAuth)
	}
	got, err := c.GetCluster(context.Background(), "c1")
	if err != nil || got.Name != "prod" {
		t.Fatalf("GetCluster: %+v err=%v", got, err)
	}
	_ = sawPath
}

func TestHTTP_MetricExportPostsToTargetPath(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b := make([]byte, r.ContentLength)
		r.Body.Read(b)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := NewHTTP(Config{Enabled: true, APIKey: "sk", BaseURL: srv.URL, HTTP: srv.Client()})
	err := c.ConfigureMetricExport(context.Background(), "c1",
		MetricExportConfig{Target: MetricExportDatadog, Settings: map[string]any{"site": "us5"}})
	if err != nil {
		t.Fatalf("ConfigureMetricExport: %v", err)
	}
	if gotPath != "/clusters/c1/metricexport/datadog" {
		t.Errorf("path = %q, want /clusters/c1/metricexport/datadog", gotPath)
	}
	if gotBody == "" || gotBody[0] != '{' {
		t.Errorf("body = %q, want JSON object", gotBody)
	}
}

func TestHTTP_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := NewHTTP(Config{Enabled: true, APIKey: "bad", BaseURL: srv.URL, HTTP: srv.Client()})
	if _, err := c.ListClusters(context.Background()); err == nil {
		t.Error("401 should surface as an error")
	}
}

// --- VClusterManager with a fake executor ---

type fakeTag string

func (f fakeTag) String() string { return string(f) }

type fakeRows struct {
	data [][2]string
	i    int
}

func (r *fakeRows) Next() bool { r.i++; return r.i <= len(r.data) }
func (r *fakeRows) Scan(dest ...any) error {
	row := r.data[r.i-1]
	*(dest[0].(*string)) = row[0]
	*(dest[1].(*string)) = row[1]
	return nil
}
func (r *fakeRows) Err() error { return nil }
func (r *fakeRows) Close()     {}

type fakeExecer struct {
	execSQL  []string
	listRows [][2]string
}

func (f *fakeExecer) Exec(_ context.Context, sql string, _ ...any) (pgconnCommandTag, error) {
	f.execSQL = append(f.execSQL, sql)
	return fakeTag("OK"), nil
}
func (f *fakeExecer) Query(_ context.Context, _ string, _ ...any) (pgxRows, error) {
	return &fakeRows{data: f.listRows}, nil
}

func TestVClusterManager_CreateDropList(t *testing.T) {
	fe := &fakeExecer{listRows: [][2]string{{"3", "analytics"}, {"4", "reporting"}}}
	m := NewVClusterManager(fe)
	ctx := context.Background()

	if err := m.Create(ctx, "analytics"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := m.Drop(ctx, "reporting"); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	if len(fe.execSQL) != 2 ||
		fe.execSQL[0] != "CREATE VIRTUAL CLUSTER analytics" ||
		fe.execSQL[1] != "DROP VIRTUAL CLUSTER reporting" {
		t.Errorf("unexpected DDL issued: %v", fe.execSQL)
	}
	vcs, err := m.List(ctx)
	if err != nil || len(vcs) != 2 || vcs[0].Name != "analytics" {
		t.Fatalf("List: %+v err=%v", vcs, err)
	}
}

func TestVClusterManager_RejectsBadName(t *testing.T) {
	m := NewVClusterManager(&fakeExecer{})
	for _, bad := range []string{"", "1abc", "drop table users;", "has space", "a--b"} {
		if err := m.Create(context.Background(), bad); err == nil {
			t.Errorf("Create(%q) should be rejected", bad)
		}
	}
}

func TestRoutingConnString(t *testing.T) {
	out, err := RoutingConnString("postgresql://root@host:26257/defaultdb?sslmode=verify-full", "analytics")
	if err != nil {
		t.Fatalf("RoutingConnString: %v", err)
	}
	u, _ := url.Parse(out)
	if got := u.Query().Get("options"); got != "-ccluster=analytics" {
		t.Errorf("options = %q, want -ccluster=analytics", got)
	}
	if u.Query().Get("sslmode") != "verify-full" {
		t.Errorf("sslmode not preserved: %q", out)
	}
	if _, err := RoutingConnString("postgresql://h/db", "bad name"); err == nil {
		t.Error("bad vcluster name should error")
	}
}

// Package cloud provides a client for the CockroachDB Cloud control-plane API
// (part of Cockroach Continuum) together with a SQL-based virtual-cluster
// lifecycle manager and a connection-routing helper.
//
// It is a separate, optional module: import it only when an application needs to
// manage Continuum control-plane resources (clusters, virtual clusters, metric
// export) programmatically. It does not change the behaviour of the core crdbgo
// transaction path.
//
// Like the aegis package, cloud keeps a simulated in-memory backend so code
// compiles and runs without a Continuum organization or API key. [New] returns
// the simulated backend whenever the client is disabled or no API key is set;
// set Config.Enabled and Config.APIKey to target the real API, which defaults to
// https://cockroachlabs.cloud/api/v1. Authentication uses a service-account
// secret key sent as a Bearer token.
package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

// DefaultBaseURL is the CockroachDB Cloud API base used when Config.BaseURL is
// empty.
const DefaultBaseURL = "https://cockroachlabs.cloud/api/v1"

// Config configures the Cloud control-plane client.
type Config struct {
	// Enabled reports whether the Cloud integration is turned on.
	Enabled bool
	// BaseURL is the Cloud API base URL. Empty selects [DefaultBaseURL].
	BaseURL string
	// APIKey is the service-account secret key, sent as a Bearer token. A blank
	// key selects the simulated in-memory backend.
	APIKey string
	// HTTP is the HTTP client used for requests. Nil selects http.DefaultClient;
	// tests inject an httptest-backed client here.
	HTTP *http.Client
}

func (c Config) useSimulated() bool { return !c.Enabled || c.APIKey == "" }

func (c Config) baseURL() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return DefaultBaseURL
}

// Cluster is a CockroachDB Cloud cluster (the control-plane view).
type Cluster struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Plan          string   `json:"plan"`
	State         string   `json:"state"`
	CloudProvider string   `json:"cloud_provider"`
	Regions       []string `json:"regions,omitempty"`
}

// VirtualCluster is a Cockroach virtual cluster that lives inside a private host
// cluster. RoutingID is the identifier used when connecting (which differs from
// the control-plane cluster ID); see [RoutingConnString].
type VirtualCluster struct {
	Name      string `json:"name"`
	ID        string `json:"id,omitempty"`
	RoutingID string `json:"routing_id,omitempty"`
	State     string `json:"state,omitempty"`
}

// MetricExportTarget identifies a Cloud metric-export sink.
type MetricExportTarget string

const (
	MetricExportDatadog    MetricExportTarget = "datadog"
	MetricExportCloudWatch MetricExportTarget = "cloudwatch"
	MetricExportPrometheus MetricExportTarget = "prometheus"
)

// MetricExportConfig configures metric export for a cluster. Settings carries
// the target-specific fields (for datadog: "site" and "api_key"; for cloudwatch:
// "role_arn", "target_region", "log_group_name"; etc.).
type MetricExportConfig struct {
	Target   MetricExportTarget
	Settings map[string]any
}

// Client is the Cloud control-plane surface used by Continuum integrations.
type Client interface {
	// ListClusters returns the clusters visible to the service account.
	ListClusters(ctx context.Context) ([]Cluster, error)
	// GetCluster returns a single cluster by its control-plane ID.
	GetCluster(ctx context.Context, clusterID string) (Cluster, error)
	// ConfigureMetricExport enables a metric-export sink for a cluster.
	ConfigureMetricExport(ctx context.Context, clusterID string, cfg MetricExportConfig) error
}

// New returns a Cloud client for cfg: the simulated backend when the client is
// disabled or has no API key, otherwise the HTTP-backed client.
func New(cfg Config) Client {
	if cfg.useSimulated() {
		return NewSimulated()
	}
	return NewHTTP(cfg)
}

// ---------------------------------------------------------------------------
// Simulated backend
// ---------------------------------------------------------------------------

// SimulatedClient is an in-memory Cloud backend for development and tests. It
// serves a small canned set of clusters and records metric-export calls.
type SimulatedClient struct {
	mu       sync.Mutex
	clusters []Cluster
	exports  map[string]MetricExportConfig
}

// NewSimulated creates a [SimulatedClient] seeded with one representative
// cluster.
func NewSimulated() *SimulatedClient {
	return &SimulatedClient{
		clusters: []Cluster{{
			ID: "00000000-0000-0000-0000-000000000001", Name: "dev-cluster",
			Plan: "STANDARD", State: "CREATED", CloudProvider: "GCP", Regions: []string{"us-east1"},
		}},
		exports: map[string]MetricExportConfig{},
	}
}

func (c *SimulatedClient) ListClusters(context.Context) ([]Cluster, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Cluster, len(c.clusters))
	copy(out, c.clusters)
	return out, nil
}

func (c *SimulatedClient) GetCluster(_ context.Context, id string) (Cluster, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, cl := range c.clusters {
		if cl.ID == id {
			return cl, nil
		}
	}
	return Cluster{}, fmt.Errorf("cloud: cluster %q not found", id)
}

func (c *SimulatedClient) ConfigureMetricExport(_ context.Context, clusterID string, cfg MetricExportConfig) error {
	if cfg.Target == "" {
		return fmt.Errorf("cloud: metric export target is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.exports[clusterID] = cfg
	return nil
}

// MetricExports returns a copy of the metric-export configs recorded so far,
// keyed by cluster ID. Useful in tests.
func (c *SimulatedClient) MetricExports() map[string]MetricExportConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]MetricExportConfig, len(c.exports))
	maps.Copy(out, c.exports)
	return out
}

var _ Client = (*SimulatedClient)(nil)

// ---------------------------------------------------------------------------
// HTTP backend (real Cloud API)
// ---------------------------------------------------------------------------

// HTTPClient talks to the real CockroachDB Cloud API using a service-account
// Bearer token.
type HTTPClient struct {
	cfg  Config
	http *http.Client
}

// NewHTTP creates an [HTTPClient] for cfg.
func NewHTTP(cfg Config) *HTTPClient {
	h := cfg.HTTP
	if h == nil {
		h = http.DefaultClient
	}
	return &HTTPClient{cfg: cfg, http: h}
}

func (c *HTTPClient) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("cloud: marshal request: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.baseURL()+path, rdr)
	if err != nil {
		return fmt.Errorf("cloud: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cloud: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("cloud: %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("cloud: decode response: %w", err)
		}
	}
	return nil
}

func (c *HTTPClient) ListClusters(ctx context.Context) ([]Cluster, error) {
	var resp struct {
		Clusters []Cluster `json:"clusters"`
	}
	if err := c.do(ctx, http.MethodGet, "/clusters", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Clusters, nil
}

func (c *HTTPClient) GetCluster(ctx context.Context, id string) (Cluster, error) {
	var cl Cluster
	if err := c.do(ctx, http.MethodGet, "/clusters/"+url.PathEscape(id), nil, &cl); err != nil {
		return Cluster{}, err
	}
	return cl, nil
}

func (c *HTTPClient) ConfigureMetricExport(ctx context.Context, clusterID string, cfg MetricExportConfig) error {
	if cfg.Target == "" {
		return fmt.Errorf("cloud: metric export target is required")
	}
	path := fmt.Sprintf("/clusters/%s/metricexport/%s", url.PathEscape(clusterID), url.PathEscape(string(cfg.Target)))
	return c.do(ctx, http.MethodPost, path, cfg.Settings, nil)
}

var _ Client = (*HTTPClient)(nil)

// ---------------------------------------------------------------------------
// Virtual-cluster lifecycle (SQL) + connection routing
// ---------------------------------------------------------------------------

// Execer is the minimal pgx-style executor the [VClusterManager] needs. Both
// *pgxpool.Pool and *pgx.Conn satisfy it, and it is trivially faked in tests.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconnCommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgxRows, error)
}

// The two tiny aliases below keep this package from importing pgx directly in
// its exported signature while still being satisfied by the pgx types. They are
// structural: any type whose methods match is accepted.
type pgconnCommandTag interface{ String() string }

type pgxRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}

// identRe allows only safe SQL identifiers for virtual-cluster names. Names are
// interpolated into DDL (identifiers cannot be bind parameters), so this guards
// against injection.
var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

func validateIdent(name string) error {
	if !identRe.MatchString(name) {
		return fmt.Errorf("cloud: invalid virtual-cluster name %q (allowed: letters, digits, underscore; must not start with a digit)", name)
	}
	return nil
}

// VClusterManager manages the lifecycle of virtual clusters over a SQL
// connection to a Continuum host cluster. The DDL statements it issues
// (CREATE/SHOW/DROP VIRTUAL CLUSTER) require a Continuum-enabled cluster.
type VClusterManager struct {
	db Execer
}

// NewVClusterManager returns a manager that runs virtual-cluster DDL over db.
func NewVClusterManager(db Execer) *VClusterManager { return &VClusterManager{db: db} }

// Create runs CREATE VIRTUAL CLUSTER for name.
func (m *VClusterManager) Create(ctx context.Context, name string) error {
	if err := validateIdent(name); err != nil {
		return err
	}
	_, err := m.db.Exec(ctx, "CREATE VIRTUAL CLUSTER "+name)
	if err != nil {
		return fmt.Errorf("cloud: create virtual cluster %q: %w", name, err)
	}
	return nil
}

// Drop runs DROP VIRTUAL CLUSTER for name.
func (m *VClusterManager) Drop(ctx context.Context, name string) error {
	if err := validateIdent(name); err != nil {
		return err
	}
	_, err := m.db.Exec(ctx, "DROP VIRTUAL CLUSTER "+name)
	if err != nil {
		return fmt.Errorf("cloud: drop virtual cluster %q: %w", name, err)
	}
	return nil
}

// List runs SHOW VIRTUAL CLUSTERS and returns the rows. The scan reads the
// (id, name) columns common to CockroachDB's SHOW VIRTUAL CLUSTERS output.
func (m *VClusterManager) List(ctx context.Context) ([]VirtualCluster, error) {
	rows, err := m.db.Query(ctx, "SHOW VIRTUAL CLUSTERS")
	if err != nil {
		return nil, fmt.Errorf("cloud: show virtual clusters: %w", err)
	}
	defer rows.Close()
	var out []VirtualCluster
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("cloud: scan virtual cluster row: %w", err)
		}
		out = append(out, VirtualCluster{ID: id, Name: name, RoutingID: name})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cloud: iterate virtual clusters: %w", err)
	}
	return out, nil
}

// RoutingConnString rewrites base to route connections to the named virtual
// cluster. CockroachDB routes to a virtual cluster via the connection option
// "-ccluster=<name>"; this sets (or replaces) that option while preserving the
// rest of the connection string.
func RoutingConnString(base, vclusterName string) (string, error) {
	if err := validateIdent(vclusterName); err != nil {
		return "", err
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("cloud: parse connection string: %w", err)
	}
	q := u.Query()
	q.Set("options", "-ccluster="+vclusterName)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

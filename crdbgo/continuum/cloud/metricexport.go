package cloud

import "context"

// Typed builders for the three metric-export sinks. Each returns a
// [MetricExportConfig] ready to pass to [Client.ConfigureMetricExport], so callers
// don't have to remember the Cloud API's field names. The raw Settings map remains
// available for sinks or fields not covered here (e.g. Azure Monitor).

// DatadogExport builds a Datadog metric-export config. site is the Datadog site
// (e.g. "US1", "EU1").
func DatadogExport(apiKey, site string) MetricExportConfig {
	return MetricExportConfig{
		Target:   MetricExportDatadog,
		Settings: map[string]any{"api_key": apiKey, "site": site},
	}
}

// CloudWatchExport builds an Amazon CloudWatch metric-export config. roleARN is the
// CockroachCloudMetricsExportRole ARN; targetRegion may be empty to use the
// cluster's region.
func CloudWatchExport(roleARN, logGroupName, targetRegion string) MetricExportConfig {
	settings := map[string]any{"role_arn": roleARN, "log_group_name": logGroupName}
	if targetRegion != "" {
		settings["target_region"] = targetRegion
	}
	return MetricExportConfig{Target: MetricExportCloudWatch, Settings: settings}
}

// PrometheusExport builds a Prometheus metric-export config. Prometheus export is
// enabled without additional settings.
func PrometheusExport() MetricExportConfig {
	return MetricExportConfig{Target: MetricExportPrometheus, Settings: map[string]any{}}
}

// Health reports whether the Cloud control plane is reachable, by issuing a
// lightweight ListClusters. A nil error means healthy; use it for readiness probes.
func Health(ctx context.Context, c Client) error {
	_, err := c.ListClusters(ctx)
	return err
}

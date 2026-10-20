package cloud

import (
	"context"
	"testing"
)

func TestMetricExportBuilders(t *testing.T) {
	dd := DatadogExport("key-1", "US1")
	if dd.Target != MetricExportDatadog || dd.Settings["api_key"] != "key-1" || dd.Settings["site"] != "US1" {
		t.Errorf("DatadogExport = %+v", dd)
	}

	cw := CloudWatchExport("arn:aws:iam::1:role/R", "crdb-logs", "us-east-1")
	if cw.Target != MetricExportCloudWatch || cw.Settings["role_arn"] != "arn:aws:iam::1:role/R" ||
		cw.Settings["log_group_name"] != "crdb-logs" || cw.Settings["target_region"] != "us-east-1" {
		t.Errorf("CloudWatchExport = %+v", cw)
	}

	// Empty target region is omitted, not sent as "".
	cw2 := CloudWatchExport("arn", "lg", "")
	if _, ok := cw2.Settings["target_region"]; ok {
		t.Errorf("empty target_region should be omitted, got %+v", cw2.Settings)
	}

	prom := PrometheusExport()
	if prom.Target != MetricExportPrometheus || len(prom.Settings) != 0 {
		t.Errorf("PrometheusExport = %+v", prom)
	}
}

func TestHealth(t *testing.T) {
	if err := Health(context.Background(), NewSimulated()); err != nil {
		t.Errorf("Health(simulated) = %v, want nil", err)
	}
}

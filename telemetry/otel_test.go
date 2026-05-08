package telemetry

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestRegisterPrometheusCollectors(t *testing.T) {
	reg := prometheus.NewRegistry()
	if err := RegisterPrometheusCollectors(reg); err != nil {
		t.Fatalf("RegisterPrometheusCollectors() error = %v", err)
	}
	if err := RegisterPrometheusCollectors(reg); err != nil {
		t.Fatalf("second RegisterPrometheusCollectors() error = %v", err)
	}

	ObserveHTTPRequest("GET", "/health", 200, time.Millisecond)
	ObserveDBQuery("query", "users", "ok", time.Millisecond)
	ObserveRedisCommand("get", "ok", time.Millisecond)

	metrics, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if len(metrics) == 0 {
		t.Fatalf("expected gathered metrics")
	}
}

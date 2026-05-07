package node

// Test-only exports of internal symbols. Kept out of the production binary by
// the _test.go suffix.

var (
	StatusPhaseAndMessage  = (*Agent).statusPhaseAndMessage
	CapacityAndMetrics     = (*Agent).capacityAndMetrics
	MetricsLoopForTest     = (*Agent).metricsLoop
	HealthCheckLoopForTest = (*Agent).ec2HealthCheckLoop
)

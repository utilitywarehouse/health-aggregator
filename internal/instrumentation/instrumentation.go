package instrumentation

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/utilitywarehouse/health-aggregator/internal/constants"
)

// Metrics contains Counters, Gauges and Histograms for health-aggregator
type Metrics struct {
	Counters   map[string]*prometheus.CounterVec
	Gauges     map[string]*prometheus.GaugeVec
	Histograms map[string]*prometheus.HistogramVec
}

// SetupMetrics returns the required guages and counters for health-aggregator
func SetupMetrics() Metrics {
	var metrics Metrics

	metrics.Counters = setupCounters()
	metrics.Gauges = setupGauges()
	metrics.Histograms = setupHistograms()

	return metrics
}

func setupCounters() map[string]*prometheus.CounterVec {

	counters := make(map[string]*prometheus.CounterVec)

	counters[constants.HealthAggregatorOutcome] = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: constants.HealthAggregatorOutcome,
		Help: "Counts health checks performed including the outcome (whether or not the healthcheck call was successful or not)",
	}, []string{constants.PerformedHealthcheckResult})

	return counters
}

func setupGauges() map[string]*prometheus.GaugeVec {

	gauges := make(map[string]*prometheus.GaugeVec)

	gauges[constants.HealthAggregatorInFlight] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: constants.HealthAggregatorInFlight,
		Help: "Records the number of health checks which are in flight at any one time",
	}, []string{})

	gauges[constants.HealthAggregatorQueuedServices] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: constants.HealthAggregatorQueuedServices,
		Help: "Records the number of services queued awaiting health agrgegator to scrape /__/health",
	}, []string{})

	return gauges
}

func setupHistograms() map[string]*prometheus.HistogramVec {

	histograms := make(map[string]*prometheus.HistogramVec)

	histograms[constants.HealthAggregatorJobDurationSeconds] = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    constants.HealthAggregatorJobDurationSeconds,
			Help:    "Health Aggregator job duration in seconds",
			Buckets: []float64{0.2, 0.5, 1, 2, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55, 60},
		},
		[]string{"job_type"},
	)

	return histograms
}

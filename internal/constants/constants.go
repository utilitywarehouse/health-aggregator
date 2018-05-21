package constants

const (
	// AppName contains the name of this application
	AppName = "health-aggregator"
	// AppDesc contains a description of the application
	AppDesc = "This app aggregates the health of apps across k8s namespaces for a cluster."
	// DefaultEnableScrape is the default value for Namespaces and Service Annotation uw.health.aggregator.enable
	DefaultEnableScrape = "true"
	// DefaultPort is the default port for Namespaces and Service Annotation uw.health.aggregator.port
	DefaultPort = "8081"
	// ServicesCollection is the name of the mongo collection that stores k8s Services alongside annotations
	ServicesCollection = "services"
	// NamespacesCollection is the name of the mongo collection that stores k8s Namespaces alongside annotations
	NamespacesCollection = "namespaces"
	// HealthchecksCollection is the name of the mongo collection that stores health check responses for k8s Services
	HealthchecksCollection = "checks"
	// DBName is the mongo database name
	DBName = "healthaggregator"
	// HealthAggregatorOutcome is the name of the metrics counter
	HealthAggregatorOutcome = "health_aggregator_outcome"
	// PerformedHealthcheckResult represents the result of the healthcheck e.g. was the healthcheck successfully called or not
	PerformedHealthcheckResult = "performed_healthcheck_result"
	// HealthAggregatorInFlight records the number of health checks which are currently in flight
	HealthAggregatorInFlight = "health_aggregator_checks_in_flight"
)

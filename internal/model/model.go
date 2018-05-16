package model

import "time"

// Service describes a k8s Service including the associated Health Aggregator configuration
type Service struct {
	Name              string            `json:"name" bson:"name"`
	Namespace         string            `json:"namespace" bson:"namespace"`
	HealthcheckURL    string            `json:"healthcheckURL" bson:"healthcheckURL"`
	HealthAnnotations HealthAnnotations `json:"healthAnnotations" bson:"healthAnnotations"`
	AppPort           string            `json:"appPort" bson:"appPort"`
	Deployment        DeployInfo        `json:"deployment" bson:"deployment"`
}

// Pod describes a k8s pod
type Pod struct {
	Name        string
	Node        string
	IP          string
	ServiceName string
}

// DeployInfo describes k8s deployment information, limited to DesiredReplicas
type DeployInfo struct {
	DesiredReplicas int32 `json:"desiredReplicas" bson:"desiredReplicas"`
}

// Namespace desribes a k8s Namespace including the associated Health Aggregator configuration
type Namespace struct {
	Name              string            `json:"name" bson:"name"`
	HealthAnnotations HealthAnnotations `json:"healthAnnotations" bson:"healthAnnotations"`
}

// HealthAnnotations matching the associated annotations against the resource in k8s
type HealthAnnotations struct {
	EnableScrape string `json:"enableScrape" bson:"enableScrape"` // k8s annotation: uw.health.aggregator.enable
	Port         string `json:"port" bson:"port"`                 // k8s annotation: uw.health.aggregator.port
}

// ServiceStatus describes the state of a service, including the results of all pods related to the service,
// the aggregated state based on those pod checks, and metadata about the service status check
type ServiceStatus struct {
	Service             Service             `json:"service" bson:"service"`
	CheckTime           time.Time           `json:"checkTime" bson:"checkTime"`
	HumanisedCheckTime  string              `json:"-"`
	AggregatedState     string              `json:"aggregatedState" bson:"aggregatedState"`
	StatePriority       int                 `json:"-"`
	StateSince          time.Time           `json:"stateSince" bson:"stateSince"`
	PreviousState       string              `json:"previousState" bson:"previousState"`
	HumanisedStateSince string              `json:"-"`
	Error               string              `json:"error" bson:"error"`
	PodChecks           []PodHealthResponse `json:"podChecks" bson:"podChecks"`
}

// PodHealthResponse describes the result of a health check for an individual pod, including
// metadata about the check made
type PodHealthResponse struct {
	Name               string          `json:"name" bson:"name"`
	CheckTime          time.Time       `json:"checkTime" bson:"checkTime"`
	HumanisedCheckTime string          `json:"-"`
	State              string          `json:"state" bson:"state"`
	StatusCode         int             `json:"statusCode" bson:"statusCode"`
	Error              string          `json:"error" bson:"error"`
	Body               HealthcheckBody `json:"body,omitempty" bson:"body"`
}

// HealthcheckBody describes the actual json response for a UW health check
type HealthcheckBody struct {
	Name        string  `json:"name,omitempty" bson:"name"`
	Description string  `json:"description,omitempty" bson:"description"`
	Health      string  `json:"health,omitempty" bson:"health"`
	Checks      []Check `json:"checks,omitempty" bson:"checks"`
}

// Check describes an individual Check for a specific health check response
type Check struct {
	Name   string `json:"name" bson:"name"`
	Health string `json:"health" bson:"health"`
	Output string `json:"output,omitempty" bson:"output"`
	Action string `json:"action,omitempty" bson:"action"`
	Impact string `json:"impact,omitempty" bson:"impact"`
}

// TemplatedChecks wraps a list of HealthcheckResp for rendering in an html template
type TemplatedChecks struct {
	Namespace string
	Checks    []ServiceStatus
}

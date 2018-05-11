package model

import "time"

// Service describes a k8s Service including the associated Health Aggregator configuration
type Service struct {
	Name              string            `json:"name" bson:"name"`
	Namespace         string            `json:"namespace" bson:"namespace"`
	HealthcheckURL    string            `json:"healthcheckURL" bson:"healthcheckURL"`
	HealthAnnotations HealthAnnotations `json:"healthAnnotations" bson:"healthAnnotations"`
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

// HealthcheckResp describes the performed health check against a Service
type HealthcheckResp struct {
	Service             Service         `json:"service" bson:"service"`
	CheckTime           time.Time       `json:"checkTime" bson:"checkTime"`
	HumanisedCheckTime  string          `json:"-"`
	State               string          `json:"state" bson:"state"`
	StatePriority       int             `json:"-"`
	StateSince          time.Time       `json:"stateSince" bson:"stateSince"`
	HumanisedStateSince string          `json:"-"`
	StatusCode          int             `json:"statusCode" bson:"statusCode"`
	Error               string          `json:"error" bson:"error"`
	Body                HealthcheckBody `json:"healthcheckBody,omitempty" bson:"healthcheckBody"`
}

// HealthcheckBody describes the actual json response for a UW health check
type HealthcheckBody struct {
	Name        string  `json:"name" bson:"name"`
	Description string  `json:"description" bson:"description"`
	Health      string  `json:"health" bson:"health"`
	Checks      []Check `json:"checks" bson:"checks"`
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
	Checks    []HealthcheckResp
}

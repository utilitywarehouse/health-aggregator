package main

import "time"

type service struct {
	Name              string            `json:"name" bson:"name"`
	Namespace         string            `json:"namespace" bson:"namespace"`
	HealthcheckURL    string            `json:"healthcheckURL" bson:"healthcheckURL"`
	HealthAnnotations healthAnnotations `json:"healthAnnotations" bson:"healthAnnotations"`
}

type namespace struct {
	Name              string            `json:"name" bson:"name"`
	HealthAnnotations healthAnnotations `json:"healthAnnotations" bson:"healthAnnotations"`
}

type healthAnnotations struct {
	EnableScrape string `json:"enableScrape" bson:"enableScrape"`
	Port         string `json:"port" bson:"port"`
}

type healthcheckResp struct {
	Service    service         `json:"service" bson:"service"`
	CheckTime  time.Time       `json:"checkTime" bson:"checkTime"`
	State      string          `json:"state" bson:"state"`
	StateSince time.Time       `json:"stateSince" bson:"stateSince"`
	StatusCode int             `json:"statusCode" bson:"statusCode"`
	Error      string          `json:"error" bson:"error"`
	Body       healthcheckBody `json:"healthcheckBody,omitempty" bson:"healthcheckBody"`
}

type healthcheckBody struct {
	Name        string  `json:"name" bson:"name"`
	Description string  `json:"description" bson:"description"`
	Health      string  `json:"health" bson:"health"`
	Checks      []check `json:"checks" bson:"checks"`
}

type check struct {
	Name   string `json:"name" bson:"name"`
	Health string `json:"health" bson:"health"`
	Output string `json:"output,omitempty" bson:"output"`
	Action string `json:"action,omitempty" bson:"action"`
	Impact string `json:"impact,omitempty" bson:"impact"`
}

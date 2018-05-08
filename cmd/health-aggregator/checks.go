package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

func getHealthchecks(restrictToNamespace string, mgoRepo *MongoRepository, healthchecks chan service, errs chan error) {
	services, err := findAllServicesWithHealthScrapeEnabled(mgoRepo, restrictToNamespace)
	if err != nil {
		select {
		case errs <- fmt.Errorf("Could not get services (%v)", err):
		default:
		}
		return
	}
	log.Debugf("Adding %v service to channel with %v elements\n", len(services), len(healthchecks))
	for _, s := range services {
		healthchecks <- s
	}
}

func removeHealthchecksOlderThan(removeAfterDays int, mgoRepo *MongoRepository, errs chan error) {
	err := deleteHealthchecksOlderThan(removeAfterDays, mgoRepo)
	if err != nil {
		select {
		case errs <- fmt.Errorf("Could not delete old healthchecks (%v)", err):
		default:
		}
		return
	}
}

type httpClient interface {
	Do(req *http.Request) (resp *http.Response, err error)
}

type healthChecker struct {
	client   httpClient
	runLocal bool
}

func newHealthChecker() healthChecker {
	return healthChecker{client: client, runLocal: outOfCluster}
}

func (c *healthChecker) doHealthchecks(healthchecks chan service, responses chan healthcheckResp, errs chan error) {
	readers := 10
	for i := 0; i < readers; i++ {
		go func(healthchecks chan service) {
			for svc := range healthchecks {
				log.Debugf("Trying %v...", svc.HealthcheckURL)
				if c.runLocal {

					errText := fmt.Sprintf("Could not get response from %v", svc.HealthcheckURL)
					time.Sleep(1 * time.Second)
					select {
					case errs <- fmt.Errorf(errText):
					default:
					}
					select {
					case responses <- healthcheckResp{Service: svc, State: "unhealthy", Body: healthcheckBody{}, StatusCode: 0, CheckTime: time.Now().UTC(), Error: errText}:
					default:
					}
					continue
				}

				req, err := http.NewRequest("GET", svc.HealthcheckURL, nil)
				if err != nil {
					errText := fmt.Sprintf("Could not get response from %v: (%v)", svc.HealthcheckURL, err)
					select {
					case errs <- fmt.Errorf(errText):
					default:
					}
					select {
					case responses <- healthcheckResp{Service: svc, State: "unhealthy", Body: healthcheckBody{}, StatusCode: 0, CheckTime: time.Now().UTC(), Error: errText}:
					default:
					}
					continue
				}
				resp, err := c.client.Do(req)
				if err != nil {
					errText := fmt.Sprintf("Could not get response from %v: (%v)", svc.HealthcheckURL, err)
					select {
					case errs <- fmt.Errorf(errText):
					default:
					}
					select {
					case responses <- healthcheckResp{Service: svc, State: "unhealthy", Body: healthcheckBody{}, StatusCode: 0, CheckTime: time.Now().UTC(), Error: errText}:
					default:
					}
					continue
				}

				if resp.StatusCode != http.StatusOK {
					errText := fmt.Sprintf("__/health returned %d for %s", resp.StatusCode, svc.HealthcheckURL)
					select {
					case errs <- fmt.Errorf(errText):
					default:
					}
					select {
					case responses <- healthcheckResp{Service: svc, State: "unhealthy", Body: healthcheckBody{}, StatusCode: resp.StatusCode, CheckTime: time.Now().UTC(), Error: errText}:
					default:
					}
					if resp != nil && resp.Body != nil {
						io.Copy(ioutil.Discard, resp.Body)
						resp.Body.Close()
					}
					continue
				}
				dec := json.NewDecoder(resp.Body)
				var checkBody healthcheckBody

				if err := dec.Decode(&checkBody); err != nil {
					errText := fmt.Sprintf("Could not json decode __/health response for %s", svc.HealthcheckURL)
					select {
					case errs <- fmt.Errorf(errText):
					default:
					}
					select {
					case responses <- healthcheckResp{Service: svc, State: "unhealthy", Body: healthcheckBody{}, StatusCode: resp.StatusCode, CheckTime: time.Now().UTC(), Error: errText}:
					default:
					}
					continue
				}
				resp.Body.Close()
				responses <- healthcheckResp{Service: svc, State: checkBody.Health, Body: checkBody, StatusCode: resp.StatusCode, CheckTime: time.Now().UTC(), Error: ""}
			}
		}(healthchecks)
	}
}

package main

import (
	"fmt"
	"net/http"
	"time"
)

func getHealthchecks(mgoRepo *MongoRepository, healthchecks chan service, errs chan error) {
	services, err := findAllServicesWithHealthScrapeEnabled(mgoRepo)
	if err != nil {
		select {
		case errs <- fmt.Errorf("Could not get services (%v)", err):
		default:
		}
		return
	}
	fmt.Printf("Adding %v service to channel with %v elements\n", len(services), len(healthchecks))
	for _, s := range services {
		healthchecks <- s
	}
}

func removeOldHealthchecks(mgoRepo *MongoRepository, errs chan error) {
	err := deleteHealthchecksOlderThan(mgoRepo, 1)
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
	client httpClient
}

func newHealthChecker() healthChecker {
	return healthChecker{client: client}
}

func (c *healthChecker) doHealthchecks(healthchecks chan service, responses chan healthcheckResp, errs chan error) {
	readers := 10
	for i := 0; i < readers; i++ {
		go func(healthchecks chan service) {
			for s := range healthchecks {
				// TEST CODE
				if s.Name == "customer-events-history-fabricator" {
					time.Sleep(time.Second * 10)
				}
				errText := fmt.Sprintf("Could not get response from %v", s.HealthcheckURL)
				time.Sleep(1 * time.Second)
				select {
				case errs <- fmt.Errorf(errText):
				default:
				}
				select {
				case responses <- healthcheckResp{Service: s, State: "unhealthy", Body: healthcheckBody{}, StatusCode: 0, CheckTime: time.Now().UTC(), Error: errText}:
				default:
				}
				continue
				// END TEST CODE

				// UNCOMMENT THIS ONCE NAMESPACES ARE ANNOTATED
				// 			req, err := http.NewRequest("GET", s.HealthcheckURL, nil)
				// 			if err != nil {
				// 				errText := fmt.Sprintf("Could not get response from %v: (%v)", s.HealthcheckURL, err)
				// 				select {
				// 				case errs <- fmt.Errorf(errText):
				// 				default:
				// 				}
				// 				select {
				// 				case responses <- healthcheckResp{Service: s, State: "unhealthy", Body: healthcheckBody{}, StatusCode: 0, CheckTime: time.Now().UTC(), Error: errText}:
				// 				default:
				// 				}
				// 				continue
				// 			}
				// 			resp, err := c.client.Do(req)
				// 			if err != nil {
				// 				errText := fmt.Sprintf("Could not get response from %v: (%v)", s.HealthcheckURL, err)
				// 				select {
				// 				case errs <- fmt.Errorf(errText):
				// 				default:
				// 				}
				// 				select {
				// 				case responses <- healthcheckResp{Service: s, State: "unhealthy", Body: healthcheckBody{}, StatusCode: 0, CheckTime: time.Now().UTC(), Error: errText}:
				// 				default:
				// 				}
				// 				continue
				// 			}

				// 			if resp.StatusCode != http.StatusOK {
				// 				errText := fmt.Sprintf("__/health returned %d for %s", resp.StatusCode, s.HealthcheckURL)
				// 				select {
				// 				case errs <- fmt.Errorf(errText):
				// 				default:
				// 				}
				// 				select {
				// 				case responses <- healthcheckResp{Service: s, State: "unhealthy", Body: healthcheckBody{}, StatusCode: resp.StatusCode, CheckTime: time.Now().UTC(), Error: errText}:
				// 				default:
				// 				}
				// 				if resp != nil && resp.Body != nil {
				// 					io.Copy(ioutil.Discard, resp.Body)
				// 					resp.Body.Close()
				// 				}
				// 				continue
				// 			}
				// 			dec := json.NewDecoder(resp.Body)
				// 			var checkBody healthcheckBody

				// 			if err := dec.Decode(&checkBody); err != nil {
				// 				errText := fmt.Sprintf("Could not json decode __/health response for %s", s.HealthcheckURL)
				// 				select {
				// 				case errs <- fmt.Errorf(errText):
				// 				default:
				// 				}
				// 				select {
				// 				case responses <- healthcheckResp{Service: s, State: "unhealthy", Body: healthcheckBody{}, StatusCode: resp.StatusCode, CheckTime: time.Now().UTC(), Error: errText}:
				// 				default:
				// 				}
				// 				continue
				// 			}
				// 			resp.Body.Close()
				// 			responses <- healthcheckResp{Service: s, State: checkBody.Health, Body: checkBody, StatusCode: resp.StatusCode, CheckTime: time.Now().UTC(), Error: ""}
			}
		}(healthchecks)
	}
}

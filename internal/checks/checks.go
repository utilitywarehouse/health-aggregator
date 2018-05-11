package checks

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/utilitywarehouse/health-aggregator/internal/model"
)

var (
	client = &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 128,
			Dial: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).Dial,
		},
	}
)

type httpClient interface {
	Do(req *http.Request) (resp *http.Response, err error)
}

// HealthChecker contains the httpClient
type HealthChecker struct {
	client httpClient
}

// NewHealthChecker returns a struct with an httpClient
func NewHealthChecker() HealthChecker {
	return HealthChecker{client: client}
}

// DoHealthchecks performs http requests to retrieve health check responses for Services on a channel of type Service.
// Responses are sent to a channel of type HealthcheckResp and any errors are sent to a channel of type error.
func (c *HealthChecker) DoHealthchecks(healthchecks chan model.Service, responses chan model.HealthcheckResp, errs chan error) {
	readers := 10
	for i := 0; i < readers; i++ {
		go func(healthchecks chan model.Service) {
			for svc := range healthchecks {
				log.Debugf("Trying %v...", svc.HealthcheckURL)

				req, err := http.NewRequest("GET", svc.HealthcheckURL, nil)
				if err != nil {
					errText := fmt.Sprintf("Could not get response from %v: (%v)", svc.HealthcheckURL, err)
					select {
					case errs <- fmt.Errorf(errText):
					default:
					}
					select {
					case responses <- model.HealthcheckResp{Service: svc, State: "unhealthy", Body: model.HealthcheckBody{}, StatusCode: 0, CheckTime: time.Now().UTC(), Error: errText}:
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
					case responses <- model.HealthcheckResp{Service: svc, State: "unhealthy", Body: model.HealthcheckBody{}, StatusCode: 0, CheckTime: time.Now().UTC(), Error: errText}:
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
					case responses <- model.HealthcheckResp{Service: svc, State: "unhealthy", Body: model.HealthcheckBody{}, StatusCode: resp.StatusCode, CheckTime: time.Now().UTC(), Error: errText}:
					default:
					}
					if resp != nil && resp.Body != nil {
						io.Copy(ioutil.Discard, resp.Body)
						resp.Body.Close()
					}
					continue
				}
				dec := json.NewDecoder(resp.Body)
				var checkBody model.HealthcheckBody

				if err := dec.Decode(&checkBody); err != nil {
					errText := fmt.Sprintf("Could not json decode __/health response for %s", svc.HealthcheckURL)
					select {
					case errs <- fmt.Errorf(errText):
					default:
					}
					select {
					case responses <- model.HealthcheckResp{Service: svc, State: "unhealthy", Body: model.HealthcheckBody{}, StatusCode: resp.StatusCode, CheckTime: time.Now().UTC(), Error: errText}:
					default:
					}
					continue
				}
				resp.Body.Close()
				responses <- model.HealthcheckResp{Service: svc, State: checkBody.Health, Body: checkBody, StatusCode: resp.StatusCode, CheckTime: time.Now().UTC(), Error: ""}
			}
		}(healthchecks)
	}
}

package statuspage

import (
	"fmt"
	"net"
	"time"

	"bytes"
	"net/http"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/utilitywarehouse/health-aggregator/internal/constants"
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

// Updater is a struct containing the page to be updated and an http client - to be used as a receiver
type Updater struct {
	StatusPageID      string
	Client            httpClient
	APIKey            string
	StatusPageBaseURL string
}

const (
	operational         = "operational"
	degradedPerformance = "degraded_performance"
	partialOutage       = "partial_outage"
	majorOutage         = "major_outage"
	underMaintenance    = "under_maintenance"
)

// NewStatusPageUpdater returns a struct with the required information to update component statuses in statuspage.io
func NewStatusPageUpdater(statusPageBaseURL string, statusPage string, apiKey string) Updater {

	return Updater{StatusPageBaseURL: statusPageBaseURL, StatusPageID: statusPage, APIKey: apiKey, Client: client}
}

// MapUWStatusToStatuspageIOStatus maps any valid UW health status to the corresponding statuspage.io status
func MapUWStatusToStatuspageIOStatus(uwStatus string) (string, error) {
	switch uwStatus {
	case constants.Unhealthy:
		return partialOutage, nil
	case constants.Healthy:
		return operational, nil
	case constants.Degraded:
		return degradedPerformance, nil
	}
	return "", fmt.Errorf("unknown uw status: %v", uwStatus)
}

// SetComponentStatus updates the status for a statuspage.io status
func (u *Updater) SetComponentStatus(component model.Component) error {

	statusIOBody := []byte(fmt.Sprintf("component[status]=%v", component.Status))

	url := fmt.Sprintf(u.StatusPageBaseURL+"/pages/%v/components/%v.json", u.StatusPageID, component.ID)

	log.Debugf("Updating status at url: %v", url)

	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewBuffer(statusIOBody))
	if err != nil {
		return fmt.Errorf("failed to create request for statuspage.io: %v", err)
	}

	req.Header.Set("Authorization", "OAuth "+u.APIKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := u.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make request to statuspage.io: %v", err)
	}

	defer func() {
		error := resp.Body.Close()
		if error != nil {
			log.Errorf("cannot close response body reader - error was: %v", error.Error())
		}
	}()

	if resp.StatusCode != 200 {
		return fmt.Errorf("received non-200 status code (%v) from statuspage.io", resp.StatusCode)
	}
	log.Debug("Successfully updated statuspage.io status")
	log.Debugf("Response: %v\n", resp.Body)

	return nil
}

// UpdateComponentStatuses takes a channel of model.Component and updates statuses at statuspage.io
func (u *Updater) UpdateComponentStatuses(components chan model.Component, errs chan error) {
	updaters := 10
	for i := 0; i < updaters; i++ {
		go func(components chan model.Component) {
			for component := range components {
				updateErr := u.SetComponentStatus(component)
				if updateErr != nil {
					select {
					case errs <- errors.Wrap(updateErr, "failed to update statuspage.io status"):
					default:
					}
				}
			}
		}(components)
	}
}

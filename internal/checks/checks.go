package checks

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/utilitywarehouse/health-aggregator/internal/constants"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/utilitywarehouse/health-aggregator/internal/model"
	"github.com/utilitywarehouse/health-aggregator/internal/statuspage"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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

// Metrics contains Counters and Gauges for this service
type Metrics struct {
	Counters map[string]*prometheus.CounterVec
	Gauges   map[string]*prometheus.GaugeVec
}

// HealthChecker contains the httpClient
type HealthChecker struct {
	client    httpClient
	k8sClient *kubernetes.Clientset
	metrics   Metrics
}

// NewHealthChecker returns a struct with an httpClient
func NewHealthChecker(k8sClient *kubernetes.Clientset, metrics Metrics) HealthChecker {

	return HealthChecker{client: client, k8sClient: k8sClient, metrics: metrics}
}

// DoHealthchecks performs http requests to retrieve health check responses for Services on a channel of type Service.
// Responses are sent to a channel of type model.ServiceStatus and any errors are sent to a channel of type error.
func (c *HealthChecker) DoHealthchecks(healthchecks chan model.Service, statusResponses chan model.ServiceStatus, statuspageIOUpdates chan model.Component, errs chan error) {
	aggregatorCounterVec := c.metrics.Counters[constants.HealthAggregatorOutcome]
	inFlightChecksGaugeVec := c.metrics.Gauges[constants.HealthAggregatorInFlight]

	readers := 10
	for i := 0; i < readers; i++ {
		go func(healthchecks chan model.Service) {
			for svc := range healthchecks {
				inFlightChecksGaugeVec.With(map[string]string{}).Inc()

				serviceCheckTime := time.Now().UTC()
				if svc.Deployment.DesiredReplicas > 0 {

					log.Debugf("Trying pod health checks for %v...", svc.Name)
					// Get the pods
					pods, err := c.getPodsForService(svc.Namespace, svc.Name)
					if err != nil {
						errText := fmt.Sprintf("cannot retrieve pods for service with name %s to perform healthcheck: %s", svc.Name, err.Error())
						select {
						case errs <- fmt.Errorf(errText):
						default:
						}
						select {
						case statusResponses <- model.ServiceStatus{Service: svc, CheckTime: time.Now().UTC(), AggregatedState: constants.Unhealthy, Error: errText}:
						default:
						}
						inFlightChecksGaugeVec.With(map[string]string{}).Dec()
						continue
					}

					// no pods are running - no point scraping the health endpoints
					if len(pods) == 0 {
						errMsg := fmt.Sprintf("desired replicas is set to %v but there are no pods running", svc.Deployment.DesiredReplicas)
						statusResponses <- model.ServiceStatus{Service: svc, CheckTime: serviceCheckTime, AggregatedState: constants.Unhealthy, Error: errMsg}
						createUpdateStatusTask(svc, constants.Unhealthy, statuspageIOUpdates)
						inFlightChecksGaugeVec.With(map[string]string{}).Dec()
						continue
					}

					noOfUnavailablePods := 0

					var podHealthResponses []model.PodHealthResponse
					for _, pod := range pods {
						var podHealthResponse model.PodHealthResponse

						podHealthResponse, err := c.getHealthCheckForPod(pod, svc.AppPort)
						if err != nil {
							if aggregatorCounterVec != nil {
								aggregatorCounterVec.With(map[string]string{constants.PerformedHealthcheckResult: "failure"}).Inc()
							}
							noOfUnavailablePods++
							log.Debugf("pod %v (service %v) health check returned an error: %v", pod.Name, pod.ServiceName, err.Error())
						} else {
							if aggregatorCounterVec != nil {
								aggregatorCounterVec.With(map[string]string{constants.PerformedHealthcheckResult: "success"}).Inc()
							}
						}

						podHealthResponses = append(podHealthResponses, podHealthResponse)
					}

					// report if there are fewer running pods than desired replicas
					var podsFewerThanDesiredReplicasMsg string
					if svc.Deployment.DesiredReplicas > int32(len(pods)) {
						podsFewerThanDesiredReplicasMsg = fmt.Sprintf("there are %v fewer running pods (%v) than the number of desired replicas (%v)", (svc.Deployment.DesiredReplicas - int32(len(pods))), len(pods), svc.Deployment.DesiredReplicas)
					}

					// report how many of the running pods are unhealthy
					var podsUnhealthyMsg string
					if int32(len(pods)-noOfUnavailablePods) > svc.Deployment.DesiredReplicas {
						podsUnhealthyMsg = fmt.Sprintf("%v/%v pods failed health checks", noOfUnavailablePods, len(pods))
					}

					switch {
					case podsFewerThanDesiredReplicasMsg != "" && podsUnhealthyMsg != "":
						statusResponses <- model.ServiceStatus{Service: svc, CheckTime: serviceCheckTime, AggregatedState: constants.Unhealthy, PodChecks: podHealthResponses, Error: podsUnhealthyMsg + " - " + podsFewerThanDesiredReplicasMsg}
						inFlightChecksGaugeVec.With(map[string]string{}).Dec()
						createUpdateStatusTask(svc, constants.Unhealthy, statuspageIOUpdates)
						continue
					case podsFewerThanDesiredReplicasMsg != "":
						statusResponses <- model.ServiceStatus{Service: svc, CheckTime: serviceCheckTime, AggregatedState: constants.Unhealthy, PodChecks: podHealthResponses, Error: podsFewerThanDesiredReplicasMsg}
						inFlightChecksGaugeVec.With(map[string]string{}).Dec()
						createUpdateStatusTask(svc, constants.Unhealthy, statuspageIOUpdates)
						continue
					case podsUnhealthyMsg != "":
						aggregatedState := mostSevereState(podHealthResponses)
						statusResponses <- model.ServiceStatus{Service: svc, CheckTime: serviceCheckTime, AggregatedState: aggregatedState, PodChecks: podHealthResponses, Error: podsUnhealthyMsg}
						inFlightChecksGaugeVec.With(map[string]string{}).Dec()
						createUpdateStatusTask(svc, aggregatedState, statuspageIOUpdates)
						continue
					default:
						aggregatedState := mostSevereState(podHealthResponses)
						statusResponses <- model.ServiceStatus{Service: svc, CheckTime: serviceCheckTime, AggregatedState: aggregatedState, PodChecks: podHealthResponses, Error: podsUnhealthyMsg}
						createUpdateStatusTask(svc, aggregatedState, statuspageIOUpdates)
					}
				}
				inFlightChecksGaugeVec.With(map[string]string{}).Dec()
			}
		}(healthchecks)
	}
}

func createUpdateStatusTask(svc model.Service, status string, statuspageIOUpdates chan model.Component) {
	// if desiredReplicas == 1 we do not have to aggregate health status and can update statuspage.io immediately
	if svc.Deployment.DesiredReplicas == 1 {
		// if there is no component id annotation, we do not attempt to update status
		if svc.ComponentID != "" {
			statuspageIOStatus, err := statuspage.MapUWStatusToStatuspageIOStatus(status)
			if err != nil {
				log.Error(errors.Wrap(err, "failed to update statuspage.io status"))
				return
			}
			select {
			case statuspageIOUpdates <- model.Component{ID: svc.ComponentID, Status: statuspageIOStatus}:
			default:
			}
		}
	}
}

func (c *HealthChecker) getPodsForService(namespaceName string, serviceName string) ([]model.Pod, error) {
	k8sPods, err := c.k8sClient.CoreV1().Pods(namespaceName).List(metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", serviceName)})
	if err != nil {
		return []model.Pod{}, fmt.Errorf("failed to get the list of pods from k8s cluster: %v", err.Error())
	}

	pods := []model.Pod{}
	for _, k8sPod := range k8sPods.Items {
		p := populatePod(k8sPod)
		pods = append(pods, p)
	}

	return pods, nil
}

func populatePod(k8sPod v1.Pod) model.Pod {
	return model.Pod{
		Name:        k8sPod.Name,
		Node:        k8sPod.Spec.NodeName,
		IP:          k8sPod.Status.PodIP,
		ServiceName: k8sPod.Labels["app"],
	}
}

func (c *HealthChecker) getHealthCheckForPod(pod model.Pod, appPort string) (model.PodHealthResponse, error) {
	log.Debugf("Getting health check for pod " + pod.Name + " service " + pod.ServiceName)
	var podHealthResponse model.PodHealthResponse
	podHealthResponse.CheckTime = time.Now().UTC()
	podHealthResponse.State = constants.Unhealthy
	podHealthResponse.StatusCode = 0
	podHealthResponse.Name = pod.Name

	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%s/__/health", pod.IP, appPort), nil)
	if err != nil {
		podHealthResponse.Error = "error constructing healthcheck request"
		return podHealthResponse, errors.New(podHealthResponse.Error + ": " + err.Error())
	}

	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		podHealthResponse.Error = "error performing healthcheck request: " + err.Error()
		return podHealthResponse, errors.New(podHealthResponse.Error + ": " + err.Error())
	}

	defer func() {
		error := resp.Body.Close()
		if error != nil {
			log.Errorf("cannot close response body reader - error was: %v", error.Error())
		}
	}()

	podHealthResponse.StatusCode = resp.StatusCode

	if resp.StatusCode != 200 {
		podHealthResponse.Error = fmt.Sprintf("healthcheck endpoint returned non-200 status (%v)", resp.StatusCode)
		return podHealthResponse, errors.New(podHealthResponse.Error)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		podHealthResponse.Error = "error reading healthcheck response"
		return podHealthResponse, errors.New(podHealthResponse.Error + ": " + err.Error())
	}

	health := &model.HealthcheckBody{}
	if err := json.Unmarshal(body, &health); err != nil {
		podHealthResponse.Error = "error parsing healthcheck response"
		return podHealthResponse, errors.New(podHealthResponse.Error + ": " + err.Error())
	}

	podHealthResponse.State = health.Health
	podHealthResponse.Body = *health

	if podHealthResponse.Body.Health != constants.Healthy {
		podHealthResponse.Error = "pod failing one or more health checks"
		return podHealthResponse, errors.New(podHealthResponse.Error)
	}

	return podHealthResponse, nil
}

func assignStatePriority(health string) int {
	switch strings.ToLower(health) {
	case constants.Unhealthy:
		return 1
	case constants.Degraded:
		return 2
	case constants.Healthy:
		return 3
	}
	return 99
}

func mostSevereState(podHealthResponses []model.PodHealthResponse) string {
	mostSevere := 99

	for _, podHealthResponse := range podHealthResponses {
		podStatePriority := assignStatePriority(podHealthResponse.State)
		if assignStatePriority(podHealthResponse.State) < mostSevere {
			mostSevere = podStatePriority
		}
	}

	switch mostSevere {
	case 1:
		return constants.Unhealthy
	case 2:
		return constants.Degraded
	case 3:
		return constants.Healthy
	}
	return "unknown"
}

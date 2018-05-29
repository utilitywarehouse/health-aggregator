package checks

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/utilitywarehouse/health-aggregator/internal/helpers"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/utilitywarehouse/health-aggregator/internal/constants"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/utilitywarehouse/health-aggregator/internal/model"
)

var (
	apiStub             *httptest.Server
	healthyCheckReponse = `{
		"name": "uw-foo",
		"description": "Performs the foo bar baz functions",
		"health": "healthy",
		"checks": [
		  {
			"name": "Database connectivity",
			"health": "healthy",
			"output": "connection to db1234.uw.systems is ok"
		  }
		]
	}`
	unhealthyCheckReponse = `{
		"name": "uw-foo",
		"description": "Performs the foo bar baz functions",
		"health": "unhealthy",
		"checks": [
		  {
			"name": "Database connectivity",
			"health": "unhealthy",
			"output": "connection to db1234.uw.systems is ok"
		  }
		]
	}`
	degradedCheckReponse = `{
		"name": "uw-foo",
		"description": "Performs the foo bar baz functions",
		"health": "degraded",
		"checks": [
		  {
			"name": "Database connectivity",
			"health": "degraded",
			"output": "connection to db1234.uw.systems is ok"
		  }
		]
	}`
)

func Test_DoHealthchecksForAHealthyService(t *testing.T) {

	errs := make(chan error, 10)
	statusResponses := make(chan model.ServiceStatus, 10)
	servicesToScrape := make(chan model.Service, 10)

	namespaceName := "energy"

	client, svc := setUpNamespaceWithService(t, namespaceName, 2)

	err := attachPods(2, namespaceName, svc.Name, client)
	require.NoError(t, err)

	setupServerReturnHealthyPod()

	checker := NewHealthChecker(client, setupMetrics(), apiStub.URL)

	go checker.DoHealthchecks(servicesToScrape, statusResponses, errs)

	// add services to scrape to channel
	go func() {
		servicesToScrape <- svc
		close(servicesToScrape)
	}()

	select {
	case <-errs:
		t.Errorf("Should not get an error")

	case s := <-statusResponses:
		assert.Equal(t, constants.Healthy, s.AggregatedState)
		assert.Equal(t, "", s.Error)
		assert.Equal(t, 2, len(s.PodChecks))
	}
}

func Test_DoHealthchecksForAnUnhealthyService(t *testing.T) {

	errs := make(chan error, 10)
	statusResponses := make(chan model.ServiceStatus, 10)
	servicesToScrape := make(chan model.Service, 10)

	namespaceName := "energy"

	client, svc := setUpNamespaceWithService(t, namespaceName, 2)

	err := attachPods(2, namespaceName, svc.Name, client)
	require.NoError(t, err)

	setupServerReturnUnhealthyPod()

	checker := NewHealthChecker(client, setupMetrics(), apiStub.URL)

	go checker.DoHealthchecks(servicesToScrape, statusResponses, errs)

	// add services to scrape to channel
	go func() {
		servicesToScrape <- svc
		close(servicesToScrape)
	}()

	select {
	case <-errs:
		t.Errorf("Should not get an error")

	case s := <-statusResponses:
		assert.Equal(t, constants.Unhealthy, s.AggregatedState)
		assert.Equal(t, "", s.Error)
		assert.Equal(t, 2, len(s.PodChecks))
	}
}

func Test_DoHealthchecksForADegradedService(t *testing.T) {

	errs := make(chan error, 10)
	statusResponses := make(chan model.ServiceStatus, 10)
	servicesToScrape := make(chan model.Service, 10)

	namespaceName := "energy"

	client, svc := setUpNamespaceWithService(t, namespaceName, 2)

	err := attachPods(2, namespaceName, svc.Name, client)
	require.NoError(t, err)

	setupServerReturnDegradedPod()

	checker := NewHealthChecker(client, setupMetrics(), apiStub.URL)

	go checker.DoHealthchecks(servicesToScrape, statusResponses, errs)

	// add services to scrape to channel
	go func() {
		servicesToScrape <- svc
		close(servicesToScrape)
	}()

	select {
	case <-errs:
		t.Errorf("Should not get an error")

	case s := <-statusResponses:
		assert.Equal(t, constants.Degraded, s.AggregatedState)
		assert.Equal(t, "", s.Error)
		assert.Equal(t, 2, len(s.PodChecks))
	}
}

func Test_DoHealthchecksWhenFewerThanDesiredPodsRunning(t *testing.T) {

	errs := make(chan error, 10)
	statusResponses := make(chan model.ServiceStatus, 10)
	servicesToScrape := make(chan model.Service, 10)

	namespaceName := "energy"

	client, svc := setUpNamespaceWithService(t, namespaceName, 2)

	err := attachPods(1, namespaceName, svc.Name, client)
	require.NoError(t, err)

	setupServerReturnHealthyPod()

	checker := NewHealthChecker(client, setupMetrics(), apiStub.URL)
	go checker.DoHealthchecks(servicesToScrape, statusResponses, errs)

	// add services to scrape to channel
	go func() {
		servicesToScrape <- svc
		close(servicesToScrape)
	}()

	select {
	case <-errs:
		t.Errorf("Should not get an error")

	case s := <-statusResponses:
		assert.Equal(t, constants.Unhealthy, s.AggregatedState)
		assert.Equal(t, "there are 1 fewer running pods (1) than the number of desired replicas (2)", s.Error)
		assert.Equal(t, 1, len(s.PodChecks))
	}
}

func Test_DoHealthchecksReportsUnhealthyWhenNoPodsRunning(t *testing.T) {

	errs := make(chan error, 10)
	statusResponses := make(chan model.ServiceStatus, 10)
	servicesToScrape := make(chan model.Service, 10)

	namespaceName := "energy"

	client, svc := setUpNamespaceWithService(t, namespaceName, 2)

	checker := NewHealthChecker(client, setupMetrics(), "")
	go checker.DoHealthchecks(servicesToScrape, statusResponses, errs)

	// add services to scrape to channel
	go func() {
		servicesToScrape <- svc
		close(servicesToScrape)
	}()

	select {
	case <-errs:
		t.Errorf("Should not get an error")

	case s := <-statusResponses:
		assert.Equal(t, constants.Unhealthy, s.AggregatedState)
		assert.Equal(t, "desired replicas is set to 2 but there are no pods running", s.Error)
		assert.Equal(t, 0, len(s.PodChecks))
	}
}

func Test_DoHealthchecksReportsUnhealthyWhenPodHealthCheckReturnServerError(t *testing.T) {

	errs := make(chan error, 10)
	statusResponses := make(chan model.ServiceStatus, 10)
	servicesToScrape := make(chan model.Service, 10)

	namespaceName := "energy"

	client, svc := setUpNamespaceWithService(t, namespaceName, 1)

	err := attachPods(1, namespaceName, svc.Name, client)
	require.NoError(t, err)

	setupServerReturnError500()

	checker := NewHealthChecker(client, setupMetrics(), apiStub.URL)
	go checker.DoHealthchecks(servicesToScrape, statusResponses, errs)

	// add services to scrape to channel
	go func() {
		servicesToScrape <- svc
		close(servicesToScrape)
	}()

	select {
	case <-errs:
		t.Errorf("Should not get an error")

	case s := <-statusResponses:
		assert.Equal(t, constants.Unhealthy, s.AggregatedState)
		assert.Equal(t, "", s.Error)
		assert.Equal(t, 1, len(s.PodChecks))
	}
}

func setUpNamespaceWithService(t *testing.T, namespaceName string, desiredReplicas int) (*fake.Clientset, model.Service) {

	svc := helpers.GenerateDummyServiceForNamespace(namespaceName, desiredReplicas)

	annotations := make(map[string]string)
	annotations["uw.health.aggregator.port"] = "8080"
	annotations["uw.health.aggregator.enable"] = "true"
	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName, Annotations: annotations}}

	svcAnnotations := make(map[string]string)
	svcAnnotations["uw.health.aggregator.port"] = "8081"
	svcAnnotations["uw.health.aggregator.enable"] = "false"
	testService := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: svc.Name, Namespace: namespaceName, Annotations: svcAnnotations}}

	client := fake.NewSimpleClientset()

	_, nsErr := client.Core().Namespaces().Create(testNamespace)
	require.NoError(t, nsErr)

	_, sErr := client.Core().Services(testNamespace.Name).Create(testService)
	require.NoError(t, sErr)

	return client, svc
}

func attachPods(numRequired int, namespaceName string, serviceName string, client *fake.Clientset) error {
	for i := 0; i < numRequired; i++ {
		_, err := client.CoreV1().Pods(namespaceName).Create(
			&v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%v-pod%v", serviceName, i),
					Namespace: namespaceName,
					Labels:    map[string]string{"app": serviceName},
				},
			})
		if err != nil {
			return err
		}
	}
	return nil
}

func setupMetrics() Metrics {
	var metrics Metrics

	metrics.Counters = setupCounters()
	metrics.Gauges = setupGauges()

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

	return gauges
}

func setupServerReturnHealthyPod() {
	apiStub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// return some json for the healthy pod
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(healthyCheckReponse))
	}))
}

func setupServerReturnUnhealthyPod() {
	apiStub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// return some json for the unhealthy pod
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(unhealthyCheckReponse))
	}))
}

func setupServerReturnDegradedPod() {
	apiStub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// return some json for the degraded pod
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(degradedCheckReponse))
	}))
}

func setupServerReturnError500() {
	apiStub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
}

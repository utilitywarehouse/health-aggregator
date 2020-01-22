package checks

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/utilitywarehouse/health-aggregator/internal/helpers"
	"github.com/utilitywarehouse/health-aggregator/internal/instrumentation"

	"github.com/stretchr/testify/require"
	"github.com/utilitywarehouse/health-aggregator/internal/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/utilitywarehouse/health-aggregator/internal/model"
)

var (
	namespaceName       = "energy"
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

	client, svc := setUpNamespaceWithService(t, 2)

	err := attachPods(2, svc.Name, client)
	require.NoError(t, err)

	setupServerReturnHealthyPod()

	checker := NewHealthChecker(client, instrumentation.SetupMetrics(), apiStub.URL)

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
		assert.Equal(t, 2, s.HealthyPods)
		assert.Equal(t, "", s.Error)
		assert.Equal(t, 2, len(s.PodChecks))
	}
}

func Test_DoHealthchecksForAnUnhealthyService(t *testing.T) {

	errs := make(chan error, 10)
	statusResponses := make(chan model.ServiceStatus, 10)
	servicesToScrape := make(chan model.Service, 10)

	client, svc := setUpNamespaceWithService(t, 2)

	err := attachPods(2, svc.Name, client)
	require.NoError(t, err)

	setupServerReturnUnhealthyPod()

	checker := NewHealthChecker(client, instrumentation.SetupMetrics(), apiStub.URL)

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
		assert.Equal(t, 0, s.HealthyPods)
		assert.Equal(t, 2, len(s.PodChecks))
	}
}

func Test_DoHealthchecksForADegradedService(t *testing.T) {

	errs := make(chan error, 10)
	statusResponses := make(chan model.ServiceStatus, 10)
	servicesToScrape := make(chan model.Service, 10)

	client, svc := setUpNamespaceWithService(t, 2)

	err := attachPods(2, svc.Name, client)
	require.NoError(t, err)

	setupServerReturnDegradedPod()

	checker := NewHealthChecker(client, instrumentation.SetupMetrics(), apiStub.URL)

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
		assert.Equal(t, 0, s.HealthyPods)
		assert.Equal(t, 2, len(s.PodChecks))
	}
}

func Test_DoHealthchecksWhenFewerThanDesiredPodsRunning(t *testing.T) {

	errs := make(chan error, 10)
	statusResponses := make(chan model.ServiceStatus, 10)
	servicesToScrape := make(chan model.Service, 10)

	client, svc := setUpNamespaceWithService(t, 2)

	err := attachPods(1, svc.Name, client)
	require.NoError(t, err)

	setupServerReturnHealthyPod()

	checker := NewHealthChecker(client, instrumentation.SetupMetrics(), apiStub.URL)
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
		assert.Equal(t, 1, s.HealthyPods)
		assert.Equal(t, 1, len(s.PodChecks))
	}
}

func Test_DoHealthchecksReportsUnhealthyWhenNoPodsRunning(t *testing.T) {

	errs := make(chan error, 10)
	statusResponses := make(chan model.ServiceStatus, 10)
	servicesToScrape := make(chan model.Service, 10)

	client, svc := setUpNamespaceWithService(t, 2)

	checker := NewHealthChecker(client, instrumentation.SetupMetrics(), "")
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
		assert.Equal(t, 0, s.HealthyPods)
		assert.Equal(t, 0, len(s.PodChecks))
	}
}

func Test_DoHealthchecksReportsUnhealthyWhenPodHealthCheckReturnServerError(t *testing.T) {

	errs := make(chan error, 10)
	statusResponses := make(chan model.ServiceStatus, 10)
	servicesToScrape := make(chan model.Service, 10)

	client, svc := setUpNamespaceWithService(t, 1)

	err := attachPods(1, svc.Name, client)
	require.NoError(t, err)

	setupServerReturnError500()

	checker := NewHealthChecker(client, instrumentation.SetupMetrics(), apiStub.URL)
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
		assert.Equal(t, 0, s.HealthyPods)
		assert.Equal(t, 1, len(s.PodChecks))
	}
}

func setUpNamespaceWithService(t *testing.T, desiredReplicas int) (*fake.Clientset, model.Service) {

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

	_, nsErr := client.CoreV1().Namespaces().Create(testNamespace)
	require.NoError(t, nsErr)

	_, sErr := client.CoreV1().Services(testNamespace.Name).Create(testService)
	require.NoError(t, sErr)

	return client, svc
}

func attachPods(numRequired int, serviceName string, client *fake.Clientset) error {
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

func setupServerReturnHealthyPod() {
	apiStub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// return some json for the healthy pod
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(healthyCheckReponse)); err != nil {
			log.Error(err)
		}
	}))
}

func setupServerReturnUnhealthyPod() {
	apiStub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// return some json for the unhealthy pod
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(unhealthyCheckReponse)); err != nil {
			log.Error(err)
		}
	}))
}

func setupServerReturnDegradedPod() {
	apiStub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// return some json for the degraded pod
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(degradedCheckReponse)); err != nil {
			log.Error(err)
		}
	}))
}

func setupServerReturnError500() {
	apiStub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
}

package helpers

import (
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/pkg/errors"

	"github.com/utilitywarehouse/health-aggregator/internal/model"
)

// CreateNamespace returns a model.Namespace with a randomly generated Name
func CreateNamespace() model.Namespace {
	return model.Namespace{
		Name: String(10),
		HealthAnnotations: model.HealthAnnotations{
			Port:         "8080",
			EnableScrape: "true",
		},
	}
}

const charset = "abcdefghijklmnopqrstuvwxyz" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var seededRand *rand.Rand = rand.New(
	rand.NewSource(time.Now().UnixNano()))

func stringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// String returns a randomly-generated string of the required length
func String(length int) string {
	return stringWithCharset(length, charset)
}

// GenerateDummyServiceStatus generates a dummy healthcheck response (model.HealthcheckResp) with either random
// state (healthy/unhealthy/degraded) or with the provided state string
func GenerateDummyServiceStatus(serviceName string, namespaceName string, podNames []string, state ...string) model.ServiceStatus {
	var healthCheck model.ServiceStatus

	// Service
	var svc model.Service
	svc.Name = serviceName
	svc.Namespace = namespaceName
	svc.HealthcheckURL = fmt.Sprintf("http://%s.%s/__/health", namespaceName, serviceName)
	svc.HealthAnnotations.EnableScrape = "true"
	svc.HealthAnnotations.Port = "3000"
	svc.AppPort = "8080"

	var deployInfo model.DeployInfo
	deployInfo.DesiredReplicas = int32(len(podNames))

	svc.Deployment = deployInfo

	healthCheck.Service = svc

	// CheckTime
	healthCheck.CheckTime = time.Now().UTC()

	// PodChecks
	var podChecks []model.PodHealthResponse
	for _, podName := range podNames {
		var podHealthResponse model.PodHealthResponse
		podHealthResponse = generateDummyPodHealthResponse(podName, state...)
		podChecks = append(podChecks, podHealthResponse)
	}
	healthCheck.PodChecks = podChecks

	// Aggregated state
	if len(state) == 1 {
		healthCheck.AggregatedState = state[0]
	} else {
		healthCheck.AggregatedState = "unhealthy"
	}

	healthCheck.Error = String(10)

	return healthCheck
}

func generateDummyPodHealthResponse(podName string, state ...string) model.PodHealthResponse {
	var podHealthResponse model.PodHealthResponse
	podHealthResponse.Name = podName
	podHealthResponse.Body = generateDummyHealthcheckBody(state...)
	podHealthResponse.Error = String(10)
	podHealthResponse.StatusCode = 200
	if len(state) == 1 {
		podHealthResponse.State = state[0]
	}
	return podHealthResponse
}

func generateDummyHealthcheckBody(state ...string) model.HealthcheckBody {
	var checkBody model.HealthcheckBody

	var health string
	if len(state) > 0 {
		health = state[0]
	} else {
		health = randomHealthState()
	}

	checkBody.Name = "Check Name " + String(10)
	checkBody.Description = "Check Description " + String(10)
	checkBody.Health = health

	var checks []model.Check
	for i := 0; i < 3; i++ {
		chk := model.Check{
			Name:   "Check name " + String(10),
			Health: health,
			Output: "Output " + String(10),
			Action: "Action " + String(10),
			Impact: "Impact " + String(10),
		}
		checks = append(checks, chk)
	}
	checkBody.Checks = checks

	return checkBody
}

func randomHealthState() string {
	states := []string{"healthy", "unhealthy", "degraded"}
	rand.Seed(time.Now().Unix())
	return states[rand.Intn(len(states))]
}

// FindHealthcheckRespByError returns the HealthcheckResp with the matching Error string from a provided
// slice of type HealthcheckResp
func FindHealthcheckRespByError(searchText string, hList []model.ServiceStatus) model.ServiceStatus {

	for _, h := range hList {
		if h.Error == searchText {
			return h
		}
	}
	return model.ServiceStatus{}
}

// FindCheckByName returns the Check with the matching Name string from a provided slice of type Check
func FindCheckByName(searchText string, cList []model.Check) model.Check {
	var chk model.Check
	for _, c := range cList {
		if c.Name == searchText {
			return c
		}
	}
	return chk
}

// FindPodCheckByName returns the Check with the matching Name string from a provided slice of type Check
func FindPodCheckByName(searchText string, pList []model.PodHealthResponse) model.PodHealthResponse {
	var chk model.PodHealthResponse
	for _, p := range pList {
		if p.Name == searchText {
			return p
		}
	}
	return chk
}

// FindNamespaceByName returns a Namespace with matching Name string from a provided slice of type Namespace
func FindNamespaceByName(searchNS model.Namespace, nsList []model.Namespace) model.Namespace {

	for _, ns := range nsList {
		if ns.Name == searchNS.Name {
			return ns
		}
	}
	return model.Namespace{}
}

// FindServiceByName returns a Service with matching Name string from a provided slice of type Service
func FindServiceByName(searchS model.Service, sList []model.Service) model.Service {

	for _, s := range sList {
		if s.Name == searchS.Name {
			return s
		}
	}
	return model.Service{}
}

// TestSliceServicesEquality tests the equality of two provided slices of type Service
func TestSliceServicesEquality(a, b []model.Service) error {

	if a == nil && b == nil {
		return nil
	}

	if a == nil || b == nil {
		return fmt.Errorf("failed check a == nil || b == nil, a==nil: %v, b==nil: %v", a == nil, b == nil)
	}

	if len(a) != len(b) {
		return fmt.Errorf("failed check len(a) != len(b) len(a): %v len(b): %v", len(a), len(b))
	}

	sort.Slice(a, func(i, j int) bool {
		return a[i].Name < a[j].Name
	})

	sort.Slice(b, func(i, j int) bool {
		return b[i].Name < b[j].Name
	})

	for i := range a {
		err := testServiceEquality(a[i], b[i])
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("comparison failed for element %v in slice of Service", i))
		}
	}

	return nil
}

func testServiceEquality(a, b model.Service) error {
	if a.Name != b.Name {
		return fmt.Errorf("a.Name != b.Name, value a: %v value b: %v", a.Name, b.Name)
	}
	if a.Namespace != b.Namespace {
		return fmt.Errorf("a.Namespace != b.Namespace, value a: %v value b: %v", a.Namespace, b.Namespace)
	}
	if a.HealthcheckURL != b.HealthcheckURL {
		return fmt.Errorf("a.HealthcheckURL != b.HealthcheckURL, value a: %v value b: %v", a.HealthcheckURL, b.HealthcheckURL)
	}
	if a.HealthAnnotations.EnableScrape != b.HealthAnnotations.EnableScrape {
		return fmt.Errorf("a.HealthAnnotations.EnableScrape != b.HealthAnnotations.EnableScrape, value a: %v value b: %v", a.HealthAnnotations.EnableScrape, b.HealthAnnotations.EnableScrape)
	}
	if a.HealthAnnotations.Port != b.HealthAnnotations.Port {
		return fmt.Errorf("a.HealthAnnotations.Port != b.HealthAnnotations.Port, value a: %v value b: %v", a.HealthAnnotations.Port, b.HealthAnnotations.Port)
	}
	if a.AppPort != b.AppPort {
		return fmt.Errorf("a.AppPort != b.AppPort, value a: %v value b: %v", a.AppPort, b.AppPort)
	}
	return nil
}

// TestSliceNamespacesEquality tests the equality of two provided slices of type Namespace
func TestSliceNamespacesEquality(a, b []model.Namespace) error {

	if a == nil && b == nil {
		return nil
	}

	if a == nil || b == nil {
		return fmt.Errorf("failed check a == nil || b == nil, a==nil: %v, b==nil: %v", a == nil, b == nil)
	}

	if len(a) != len(b) {
		return fmt.Errorf("failed check len(a) != len(b) len(a): %v len(b): %v", len(a), len(b))
	}

	sort.Slice(a, func(i, j int) bool {
		return a[i].Name < a[j].Name
	})

	sort.Slice(b, func(i, j int) bool {
		return b[i].Name < b[j].Name
	})

	for i := range a {
		if a[i].Name != b[i].Name {
			return fmt.Errorf("a[%v].Name != b[%v].Name, value a: %v value b: %v", i, i, a[i].Name, b[i].Name)
		}
		if a[i].HealthAnnotations.EnableScrape != b[i].HealthAnnotations.EnableScrape {
			return fmt.Errorf("a[%v].HealthAnnotations.EnableScrape != b[%v].HealthAnnotations.EnableScrape, value a: %v value b: %v", i, i, a[i].HealthAnnotations.EnableScrape, b[i].HealthAnnotations.EnableScrape)
		}
		if a[i].HealthAnnotations.Port != b[i].HealthAnnotations.Port {
			return fmt.Errorf("a[%v].HealthAnnotations.Port != b[%v].HealthAnnotations.Port, value a: %v value b: %v", i, i, a[i].HealthAnnotations.Port, b[i].HealthAnnotations.Port)
		}
	}

	return nil
}

// TestServiceStatusesEquality tests the equality of two provided slices of type ServiceStatus
func TestServiceStatusesEquality(a, b []model.ServiceStatus) error {

	if a == nil && b == nil {
		return nil
	}

	if a == nil || b == nil {
		return fmt.Errorf("failed check ([]model.ServiceStatus) a == nil || b == nil, a==nil: %v, b==nil: %v", a == nil, b == nil)
	}

	if len(a) != len(b) {
		return fmt.Errorf("failed check ([]model.ServiceStatus) len(a) != len(b) len(a): %v len(b): %v", len(a), len(b))
	}

	sort.Slice(a, func(i, j int) bool {
		return a[i].Error < a[j].Error
	})

	sort.Slice(b, func(i, j int) bool {
		return b[i].Error < b[j].Error
	})

	for i := range a {
		err := testServiceEquality(a[i].Service, b[i].Service)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("Service comparison failed for element %v in slice of ServiceStatus", i))
		}
		if a[i].CheckTime.Format("2006-01-02T15:04:05.000Z") != b[i].CheckTime.Format("2006-01-02T15:04:05.000Z") {
			return fmt.Errorf("a[%v].CheckTime.Format(\"2006-01-02T15:04:05.000Z\") != b[%v].CheckTime.Format(\"2006-01-02T15:04:05.000Z\"), value a: %v value b: %v", i, i, a[i].CheckTime.Format("2006-01-02T15:04:05.000Z"), b[i].CheckTime.Format("2006-01-02T15:04:05.000Z"))
		}
		if a[i].AggregatedState != b[i].AggregatedState {
			return fmt.Errorf("a[%v].AggregatedState != b[%v].AggregatedState, value a: %v value b: %v", i, i, a[i].AggregatedState, b[i].AggregatedState)
		}
		if a[i].Error != b[i].Error {
			return fmt.Errorf("a[%v].Error != b[%v].Error, value a: %v value b: %v", i, i, a[i].Error, b[i].Error)
		}
		podCheckErr := testPodChecksEquality(a[i].PodChecks, b[i].PodChecks)
		if podCheckErr != nil {
			return errors.Wrap(err, fmt.Sprintf("PodChecks comparison failed for element %v in slice of ServiceStatus", i))
		}
	}

	return nil
}

func testPodChecksEquality(a, b []model.PodHealthResponse) error {
	if a == nil && b == nil {
		return nil
	}

	if a == nil || b == nil {
		return fmt.Errorf("failed check ([]model.PodHealthResponse) a == nil || b == nil, a==nil: %v, b==nil: %v", a == nil, b == nil)
	}

	if len(a) != len(b) {
		return fmt.Errorf("failed check ([]model.PodHealthResponse) len(a) != len(b) len(a): %v len(b): %v", len(a), len(b))
	}

	sort.Slice(a, func(i, j int) bool {
		return a[i].Error < a[j].Error
	})

	sort.Slice(b, func(i, j int) bool {
		return b[i].Error < b[j].Error
	})

	for i := 0; i < len(a); i++ {
		err := testPodCheckEquality(a[i], b[i])
		if err != nil {
			return err
		}
	}
	return nil
}

func testPodCheckEquality(a, b model.PodHealthResponse) error {
	if a.Name != b.Name {
		return fmt.Errorf("a.Name != b.Name, value a: %v value b: %v", a.Name, b.Name)
	}
	if a.CheckTime.Format("2006-01-02T15:04:05.000Z") != b.CheckTime.Format("2006-01-02T15:04:05.000Z") {
		return fmt.Errorf("a.CheckTime.Format(\"2006-01-02T15:04:05.000Z\") != b.CheckTime.Format(\"2006-01-02T15:04:05.000Z\"), value a: %v value b: %v", a.CheckTime.Format("2006-01-02T15:04:05.000Z"), b.CheckTime.Format("2006-01-02T15:04:05.000Z"))
	}
	if a.State != b.State {
		return fmt.Errorf("a.State != b.State, value a: %v value b: %v", a.State, b.State)
	}
	if a.StatusCode != b.StatusCode {
		return fmt.Errorf("a.StatusCode != b.StatusCode, value a: %v value b: %v", a.StatusCode, b.StatusCode)
	}
	if a.Error != b.Error {
		return fmt.Errorf("a.Error != b.Error, value a: %v value b: %v", a.Error, b.Error)
	}

	err := testHealthCheckBodyEquality(a.Body, b.Body)
	if err != nil {
		return errors.Wrap(err, "HealthcheckBody comparison failed")
	}
	return nil
}

func testHealthCheckBodyEquality(a, b model.HealthcheckBody) error {
	if a.Name != b.Name {
		return fmt.Errorf("a.Name != b.Name, value a: %v value b: %v", a.Name, b.Name)
	}
	if a.Description != b.Description {
		return fmt.Errorf("a.Description != b.Description, value a: %v value b: %v", a.Description, b.Description)
	}
	if a.Health != b.Health {
		return fmt.Errorf("a.Health != b.Health, value a: %v value b: %v", a.Health, b.Health)
	}

	err := testChecksEquality(a.Checks, b.Checks)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Checks comparison failed for Health Check %v", a.Name))
	}
	return nil
}

func testChecksEquality(a, b []model.Check) error {
	if a == nil && b == nil {
		return nil
	}

	if a == nil || b == nil {
		return fmt.Errorf("failed check ([]model.Check) a == nil || b == nil, a==nil: %v, b==nil: %v", a == nil, b == nil)
	}

	if len(a) != len(b) {
		return fmt.Errorf("failed check ([]model.Check) len(a) != len(b) len(a): %v len(b): %v", len(a), len(b))
	}

	sort.Slice(a, func(i, j int) bool {
		return a[i].Name < a[j].Name
	})

	sort.Slice(b, func(i, j int) bool {
		return b[i].Name < b[j].Name
	})

	for i := 0; i < len(a); i++ {
		err := testCheckEquality(a[i], b[i])
		if err != nil {
			return err
		}
	}
	return nil
}

func testCheckEquality(a, b model.Check) error {
	if a.Name != b.Name {
		return fmt.Errorf("a.Name != b.Name, value a: %v value b: %v", a.Name, b.Name)
	}
	if a.Health != b.Health {
		return fmt.Errorf("a.Health != b.Health, value a: %v value b: %v", a.Health, b.Health)
	}
	if a.Output != b.Output {
		return fmt.Errorf("a.Output != b.Output, value a: %v value b: %v", a.Output, b.Output)
	}
	if a.Action != b.Action {
		return fmt.Errorf("a.Action != b.Action, value a: %v value b: %v", a.Action, b.Action)
	}
	if a.Impact != b.Impact {
		return fmt.Errorf("a.Impact != b.Impact, value a: %v value b: %v", a.Impact, b.Impact)
	}

	return nil
}

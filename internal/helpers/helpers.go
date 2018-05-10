package helpers

import (
	"fmt"
	"math/rand"
	"sort"
	"time"

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

// GenerateDummyCheck generates a dummy healthcheck response (model.HealthcheckResp) with either random
// state (healthy/unhealthy/degraded) or with the provided state string
func GenerateDummyCheck(serviceName string, namespaceName string, state ...string) model.HealthcheckResp {
	var healthCheck model.HealthcheckResp

	var svc model.Service
	svc.Name = serviceName
	svc.Namespace = namespaceName
	svc.HealthcheckURL = fmt.Sprintf("http://%s.%s/__/health", namespaceName, serviceName)
	svc.HealthAnnotations.EnableScrape = "true"
	svc.HealthAnnotations.Port = "3000"
	healthCheck.Service = svc

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

	healthCheck.Body = checkBody
	healthCheck.CheckTime = time.Now().UTC()
	healthCheck.State = health
	healthCheck.Error = String(10)
	healthCheck.StatusCode = 500
	return healthCheck
}

func randomHealthState() string {
	states := []string{"healthy", "unhealthy", "degraded"}
	rand.Seed(time.Now().Unix())
	return states[rand.Intn(len(states))]
}

// FindHealthcheckRespByError returns the HealthcheckResp with the matching Error string from a provided
// slice of type HealthcheckResp
func FindHealthcheckRespByError(searchText string, hList []model.HealthcheckResp) model.HealthcheckResp {

	for _, h := range hList {
		if h.Error == searchText {
			return h
		}
	}
	return model.HealthcheckResp{}
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
func TestSliceServicesEquality(a, b []model.Service) bool {

	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	if len(a) != len(b) {
		return false
	}

	sort.Slice(a, func(i, j int) bool {
		return a[i].Name < a[j].Name
	})

	sort.Slice(b, func(i, j int) bool {
		return b[i].Name < b[j].Name
	})

	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
		if a[i].Namespace != b[i].Namespace {
			return false
		}
		if a[i].HealthcheckURL != b[i].HealthcheckURL {
			return false
		}
		if a[i].HealthAnnotations.EnableScrape != b[i].HealthAnnotations.EnableScrape {
			return false
		}
		if a[i].HealthAnnotations.Port != b[i].HealthAnnotations.Port {
			return false
		}
	}

	return true
}

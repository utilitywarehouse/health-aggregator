package handlers

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/globalsign/mgo"
	"github.com/gorilla/mux"
	"github.com/pborman/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/utilitywarehouse/health-aggregator/internal/db"
	"github.com/utilitywarehouse/health-aggregator/internal/helpers"
	"github.com/utilitywarehouse/health-aggregator/internal/helpers/dbutils"
	"github.com/utilitywarehouse/health-aggregator/internal/model"
)

const (
	dbURL = "localhost:27017"
)

type TestSuite struct {
	repo *db.MongoRepository
}

var s TestSuite

func TestMain(m *testing.M) {
	sess, err := mgo.Dial(dbURL)
	if err != nil {
		log.Fatalf("failed to create mongo session: %s", err.Error())
	}
	defer sess.Close()

	s.repo = db.NewMongoRepository(sess, uuid.New())

	code := m.Run()
	dbErr := s.repo.Session.DB(s.repo.DBName).DropDatabase()
	if dbErr != nil {
		log.Printf("Failed to drop database %v", s.repo.DBName)
	}
	os.Exit(code)
}
func Test_GetAllNamespacesReturnsEmptyListWhenDBEmpty(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()

	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getAllNamespaces(repoCopy))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var returnedNamespaces []model.Namespace
	jsonErr := json.Unmarshal(body, &returnedNamespaces)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}
	assert.Equal(t, len([]model.Namespace{}), len(returnedNamespaces))
}

func Test_GetAllNamespaces(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()

	ns1 := model.Namespace{Name: helpers.String(10), HealthAnnotations: model.HealthAnnotations{Port: "8080", EnableScrape: "true"}}
	ns2 := model.Namespace{Name: helpers.String(10), HealthAnnotations: model.HealthAnnotations{Port: "8081", EnableScrape: "false"}}
	dbutils.InsertItem(repoCopy, ns1)
	dbutils.InsertItem(repoCopy, ns2)
	createdNamespaces := []model.Namespace{ns1, ns2}

	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getAllNamespaces(repoCopy))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var returnedNamespaces []model.Namespace
	jsonErr := json.Unmarshal(body, &returnedNamespaces)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}

	for _, ns := range createdNamespaces {
		assert.Equal(t, ns.Name, helpers.FindNamespaceByName(ns, returnedNamespaces).Name)
		assert.Equal(t, ns.HealthAnnotations.EnableScrape, helpers.FindNamespaceByName(ns, returnedNamespaces).HealthAnnotations.EnableScrape)
		assert.Equal(t, ns.HealthAnnotations.Port, helpers.FindNamespaceByName(ns, returnedNamespaces).HealthAnnotations.Port)
	}
}

func Test_GetAllServicesReturnsEmptyListWhenDBEmpty(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()

	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getAllServices(repoCopy))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var returnedServices []model.Service
	jsonErr := json.Unmarshal(body, &returnedServices)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}

	assert.Equal(t, len([]model.Service{}), len(returnedServices))
}
func Test_GetAllServices(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()

	s1 := model.Service{Name: helpers.String(10), Namespace: helpers.String(10), HealthAnnotations: model.HealthAnnotations{Port: "8080", EnableScrape: "true"}}
	s2 := model.Service{Name: helpers.String(10), Namespace: helpers.String(10), HealthAnnotations: model.HealthAnnotations{Port: "8081", EnableScrape: "false"}}
	dbutils.InsertItem(repoCopy, s1)
	dbutils.InsertItem(repoCopy, s2)
	createdServices := []model.Service{s1, s2}

	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getAllServices(repoCopy))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var returnedServices []model.Service
	jsonErr := json.Unmarshal(body, &returnedServices)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}

	for _, s := range createdServices {
		assert.Equal(t, s.Name, helpers.FindServiceByName(s, returnedServices).Name)
		assert.Equal(t, s.Namespace, helpers.FindServiceByName(s, returnedServices).Namespace)
		assert.Equal(t, s.HealthAnnotations.EnableScrape, helpers.FindServiceByName(s, returnedServices).HealthAnnotations.EnableScrape)
		assert.Equal(t, s.HealthAnnotations.Port, helpers.FindServiceByName(s, returnedServices).HealthAnnotations.Port)
	}
}

func Test_GetServicesForNamespace(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()
	ns1 := helpers.String(10)
	ns2 := helpers.String(10)
	s1 := model.Service{Name: helpers.String(10), Namespace: ns1, HealthAnnotations: model.HealthAnnotations{Port: "8080", EnableScrape: "true"}}
	s2 := model.Service{Name: helpers.String(10), Namespace: ns1, HealthAnnotations: model.HealthAnnotations{Port: "8081", EnableScrape: "false"}}
	s3 := model.Service{Name: helpers.String(10), Namespace: ns2, HealthAnnotations: model.HealthAnnotations{Port: "8081", EnableScrape: "false"}}
	dbutils.InsertItem(repoCopy, s1)
	dbutils.InsertItem(repoCopy, s2)
	dbutils.InsertItem(repoCopy, s3)
	ns1Services := []model.Service{s1, s2}

	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getServicesForNameSpace(repoCopy))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)
	req = mux.SetURLVars(req, map[string]string{"namespace": ns1})

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var returnedServices []model.Service
	jsonErr := json.Unmarshal(body, &returnedServices)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}

	assert.Equal(t, len(ns1Services), len(returnedServices))

	for _, s := range ns1Services {
		assert.Equal(t, s.Name, helpers.FindServiceByName(s, returnedServices).Name)
		assert.Equal(t, s.Namespace, helpers.FindServiceByName(s, returnedServices).Namespace)
		assert.Equal(t, s.HealthAnnotations.EnableScrape, helpers.FindServiceByName(s, returnedServices).HealthAnnotations.EnableScrape)
		assert.Equal(t, s.HealthAnnotations.Port, helpers.FindServiceByName(s, returnedServices).HealthAnnotations.Port)
	}
}

func Test_GetServicesForNamespaceReturnsEmptyListWhenNoneExist(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()
	ns := helpers.String(10)

	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getServicesForNameSpace(repoCopy))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)
	req = mux.SetURLVars(req, map[string]string{"namespace": ns})

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var returnedServices []model.Service
	jsonErr := json.Unmarshal(body, &returnedServices)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}
	assert.Equal(t, len([]model.Service{}), len(returnedServices))
}
func Test_GetAllChecksForServiceReturnsEmptyListWhenNoneExist(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()
	svc := helpers.String(10)

	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getAllChecksForService(repoCopy))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)
	req = mux.SetURLVars(req, map[string]string{"service": svc})

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var returnedChecks []model.ServiceStatus
	jsonErr := json.Unmarshal(body, &returnedChecks)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}
	assert.Equal(t, len([]model.ServiceStatus{}), len(returnedChecks))
}
func Test_GetAllChecksForService(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()

	// Generate some service and namespace names
	ns1Name := helpers.String(10)
	ns2Name := helpers.String(10)
	s1Name := helpers.String(10)
	s2Name := helpers.String(10)

	s1Pods := []string{"s1Pod1", "s1Pod2"}
	s2Pods := []string{"s2Pod1", "s2Pod2"}

	// Create checks for a single service in a specific namespace
	chk1 := helpers.GenerateDummyServiceStatus(s1Name, ns1Name, s1Pods)
	chk2 := helpers.GenerateDummyServiceStatus(s1Name, ns1Name, s1Pods)

	// Create a check against a different service in the same namespace
	chk3 := helpers.GenerateDummyServiceStatus(s2Name, ns1Name, s2Pods)

	// Create a check against a service with the same name, but within a different namespace
	chk4 := helpers.GenerateDummyServiceStatus(s1Name, ns2Name, s1Pods)
	dbutils.InsertItems(repoCopy, chk1, chk2, chk3, chk4)

	// We only expect checks returned for a specific service within a specific namespace
	expectedHealthChecks := []model.ServiceStatus{chk1, chk2}

	// Make the request
	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getAllChecksForService(repoCopy))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)
	req = mux.SetURLVars(req, map[string]string{"namespace": ns1Name, "service": s1Name})

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	// Get the returned health check response
	var returnedHealthChecks []model.ServiceStatus
	jsonErr := json.Unmarshal(body, &returnedHealthChecks)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}

	assertEqual(t, expectedHealthChecks, returnedHealthChecks)
}

func Test_GetLatestChecksForNamespaceJSONReturnsEmptyListWhenNoneExist(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()
	ns := helpers.String(10)

	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getLatestChecksForNamespace(repoCopy))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)
	req = mux.SetURLVars(req, map[string]string{"namespace": ns})
	req.Header.Set("Accept", "application/json")

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var returnedChecks []model.ServiceStatus
	jsonErr := json.Unmarshal(body, &returnedChecks)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}
	assert.Equal(t, len([]model.ServiceStatus{}), len(returnedChecks))
	assert.Equal(t, "application/json; charset=utf-8", resp.Header().Get("Content-Type"))
}

func Test_GetLatestChecksForNamespaceHTMLReturnsMessageWhenNoneExist(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()
	ns := helpers.String(10)

	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getLatestChecksForNamespace(repoCopy))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)
	req = mux.SetURLVars(req, map[string]string{"namespace": ns})

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	assert.Equal(t, body, []byte("No checks available"))
	assert.Equal(t, resp.Header().Get("Content-Type"), "text/html; charset=utf-8")
}

func Test_GetLatestChecksForNamespaceJSON(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()

	// Generate some service and namespace names
	ns1Name := helpers.String(10)
	ns2Name := helpers.String(10)
	s1Name := helpers.String(10)
	s2Name := helpers.String(10)

	s1Pods := []string{"s1Pod1", "s1Pod2"}
	s2Pods := []string{"s2Pod1", "s2Pod2"}

	// Create checks for a single service in a specific namespace
	chk1 := helpers.GenerateDummyServiceStatus(s1Name, ns1Name, s1Pods)
	time.Sleep(time.Millisecond * 200)
	chk2 := helpers.GenerateDummyServiceStatus(s1Name, ns1Name, s1Pods)

	// Create checks against a different service in the same namespace
	chk3 := helpers.GenerateDummyServiceStatus(s2Name, ns1Name, s2Pods)
	time.Sleep(time.Millisecond * 200)
	chk4 := helpers.GenerateDummyServiceStatus(s2Name, ns1Name, s2Pods)

	// Create a check against a service with the same name, but within a different namespace
	chk5 := helpers.GenerateDummyServiceStatus(s1Name, ns2Name, s1Pods)
	dbutils.InsertItems(repoCopy, chk1, chk2, chk3, chk4, chk5)

	// We only expect the LATEST checks to be returned, and only the ones for this namespace
	expectedHealthChecks := []model.ServiceStatus{chk2, chk4}

	// Make the request
	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getLatestChecksForNamespace(repoCopy))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)
	req = mux.SetURLVars(req, map[string]string{"namespace": ns1Name})
	req.Header.Set("Accept", "application/json")

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	// Get the returned health check response
	var returnedHealthChecks []model.ServiceStatus
	jsonErr := json.Unmarshal(body, &returnedHealthChecks)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}

	assertEqual(t, expectedHealthChecks, returnedHealthChecks)
}

func Test_EnrichChecksData(t *testing.T) {

	pods := []string{"pod1"}
	healthyCheck := helpers.GenerateDummyServiceStatus("healthy-service", "a-namespace", pods, "healthy")
	degradedCheck := helpers.GenerateDummyServiceStatus("degraded-service", "a-namespace", pods, "degraded")
	unhealthyCheck := helpers.GenerateDummyServiceStatus("unhealthy-service", "a-namespace", pods, "unhealthy")

	// Set times to something predictable
	healthyCheck.CheckTime = time.Now().AddDate(0, 0, -7)  // 1 week ago
	healthyCheck.StateSince = time.Now().AddDate(0, 0, -3) // 3 days ago

	checks := []model.ServiceStatus{healthyCheck, degradedCheck, unhealthyCheck}

	enrichChecksData(checks)

	assert.Equal(t, 3, checks[0].StatePriority)
	assert.Equal(t, 2, checks[1].StatePriority)
	assert.Equal(t, 1, checks[2].StatePriority)

	assert.Equal(t, "1 week ago", checks[0].HumanisedCheckTime)
	assert.Equal(t, "3 days ago", checks[0].HumanisedStateSince)
}

func assertEqual(t *testing.T, expectedHealthChecks []model.ServiceStatus, actualHealthChecks []model.ServiceStatus) {
	require.Equal(t, len(expectedHealthChecks), len(actualHealthChecks))

	for _, expHC := range expectedHealthChecks {
		actualHealthCheck := helpers.FindHealthcheckRespByError(expHC.Error, actualHealthChecks)
		assert.Equal(t, expHC.Error, actualHealthCheck.Error)
		assert.Equal(t, expHC.CheckTime.Format("2006-01-02T15:04:05.000Z"), actualHealthCheck.CheckTime.Format("2006-01-02T15:04:05.000Z")) // Formatted since mongo returns timestamps with truncated accuracy

		assert.Equal(t, expHC.Service.Name, actualHealthCheck.Service.Name)
		assert.Equal(t, expHC.Service.Namespace, actualHealthCheck.Service.Namespace)
		assert.Equal(t, expHC.Service.HealthcheckURL, actualHealthCheck.Service.HealthcheckURL)
		assert.Equal(t, expHC.Service.HealthAnnotations.Port, actualHealthCheck.Service.HealthAnnotations.Port)
		assert.Equal(t, expHC.Service.HealthAnnotations.EnableScrape, actualHealthCheck.Service.HealthAnnotations.EnableScrape)
		assert.Equal(t, expHC.Service.AppPort, actualHealthCheck.Service.AppPort)
		assert.Equal(t, expHC.Service.Deployment.DesiredReplicas, actualHealthCheck.Service.Deployment.DesiredReplicas)

		for _, expPC := range expHC.PodChecks {
			actualPodCheck := helpers.FindPodCheckByName(expPC.Name, actualHealthCheck.PodChecks)
			assert.Equal(t, expPC.Name, actualPodCheck.Name)
			assert.Equal(t, expPC.CheckTime.Format("2006-01-02T15:04:05.000Z"), actualPodCheck.CheckTime.Format("2006-01-02T15:04:05.000Z")) // Formatted since mongo returns timestamps with truncated accuracy
			assert.Equal(t, expPC.State, actualPodCheck.State)
			assert.Equal(t, expPC.StatusCode, actualPodCheck.StatusCode)
			assert.Equal(t, expPC.Error, actualPodCheck.Error)

			assert.Equal(t, expPC.Body.Name, actualPodCheck.Body.Name)
			assert.Equal(t, expPC.Body.Health, actualPodCheck.Body.Health)
			assert.Equal(t, expPC.Body.Description, actualPodCheck.Body.Description)

			for _, expC := range expPC.Body.Checks {
				actualCheck := helpers.FindCheckByName(expC.Name, actualPodCheck.Body.Checks)
				assert.Equal(t, expC.Action, actualCheck.Action)
				assert.Equal(t, expC.Health, actualCheck.Health)
				assert.Equal(t, expC.Impact, actualCheck.Impact)
				assert.Equal(t, expC.Name, actualCheck.Name)
				assert.Equal(t, expC.Output, actualCheck.Output)
			}
		}
	}
}

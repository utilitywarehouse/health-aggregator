package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
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
)

const (
	dbURL = "localhost:27017"
)

type HandlersSuite struct {
	repo *MongoRepository
}

var s HandlersSuite

func TestMain(m *testing.M) {
	sess, err := mgo.DialWithTimeout(dbURL, 1*time.Second)
	if err != nil {
		log.Fatalf("failed to create mongo session: %s", err.Error())
	}
	defer sess.Close()

	s.repo = NewMongoRepository(sess, uuid.New())

	code := m.Run()
	dbErr := s.repo.session.DB(s.repo.dbName).DropDatabase()
	if dbErr != nil {
		log.Printf("Failed to drop database %v", s.repo.dbName)
	}
	os.Exit(code)
}
func TestGetAllNamespacesReturnsEmptyListWhenDBEmpty(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()

	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getAllNamespaces(s.repo))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var returnedNamespaces []namespace
	jsonErr := json.Unmarshal(body, &returnedNamespaces)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}
	assert.Equal(t, len([]namespace{}), len(returnedNamespaces))
}

func TestGetAllNamespaces(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()

	ns1 := namespace{Name: String(10), HealthAnnotations: healthAnnotations{Port: "8080", EnableScrape: "true"}}
	ns2 := namespace{Name: String(10), HealthAnnotations: healthAnnotations{Port: "8081", EnableScrape: "false"}}
	insertItem(s.repo, ns1)
	insertItem(s.repo, ns2)
	createdNamespaces := []namespace{ns1, ns2}

	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getAllNamespaces(s.repo))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var returnedNamespaces []namespace
	jsonErr := json.Unmarshal(body, &returnedNamespaces)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}

	for _, ns := range createdNamespaces {
		assert.Equal(t, ns.Name, findNamespaceByName(ns, returnedNamespaces).Name)
		assert.Equal(t, ns.HealthAnnotations.EnableScrape, findNamespaceByName(ns, returnedNamespaces).HealthAnnotations.EnableScrape)
		assert.Equal(t, ns.HealthAnnotations.Port, findNamespaceByName(ns, returnedNamespaces).HealthAnnotations.Port)
	}
}

func TestGetAllServicesReturnsEmptyListWhenDBEmpty(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()

	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getAllServices(s.repo))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var returnedServices []service
	jsonErr := json.Unmarshal(body, &returnedServices)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}
	assert.Equal(t, len([]service{}), len(returnedServices))
}
func TestGetAllServices(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()

	s1 := service{Name: String(10), Namespace: String(10), HealthAnnotations: healthAnnotations{Port: "8080", EnableScrape: "true"}}
	s2 := service{Name: String(10), Namespace: String(10), HealthAnnotations: healthAnnotations{Port: "8081", EnableScrape: "false"}}
	insertItem(s.repo, s1)
	insertItem(s.repo, s2)
	createdServices := []service{s1, s2}

	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getAllServices(s.repo))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var returnedServices []service
	jsonErr := json.Unmarshal(body, &returnedServices)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}

	for _, s := range createdServices {
		assert.Equal(t, s.Name, findServiceByName(s, returnedServices).Name)
		assert.Equal(t, s.Namespace, findServiceByName(s, returnedServices).Namespace)
		assert.Equal(t, s.HealthAnnotations.EnableScrape, findServiceByName(s, returnedServices).HealthAnnotations.EnableScrape)
		assert.Equal(t, s.HealthAnnotations.Port, findServiceByName(s, returnedServices).HealthAnnotations.Port)
	}
}

func TestGetServicesForNamespace(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()
	ns1 := String(10)
	ns2 := String(10)
	s1 := service{Name: String(10), Namespace: ns1, HealthAnnotations: healthAnnotations{Port: "8080", EnableScrape: "true"}}
	s2 := service{Name: String(10), Namespace: ns1, HealthAnnotations: healthAnnotations{Port: "8081", EnableScrape: "false"}}
	s3 := service{Name: String(10), Namespace: ns2, HealthAnnotations: healthAnnotations{Port: "8081", EnableScrape: "false"}}
	insertItem(s.repo, s1)
	insertItem(s.repo, s2)
	insertItem(s.repo, s3)
	ns1Services := []service{s1, s2}

	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getServicesForNameSpace(s.repo))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)
	req = mux.SetURLVars(req, map[string]string{"namespace": ns1})

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var returnedServices []service
	jsonErr := json.Unmarshal(body, &returnedServices)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}

	assert.Equal(t, len(ns1Services), len(returnedServices))

	for _, s := range ns1Services {
		assert.Equal(t, s.Name, findServiceByName(s, returnedServices).Name)
		assert.Equal(t, s.Namespace, findServiceByName(s, returnedServices).Namespace)
		assert.Equal(t, s.HealthAnnotations.EnableScrape, findServiceByName(s, returnedServices).HealthAnnotations.EnableScrape)
		assert.Equal(t, s.HealthAnnotations.Port, findServiceByName(s, returnedServices).HealthAnnotations.Port)
	}
}

func TestGetServicesForNamespaceReturnsEmptyListWhenNoneExist(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()
	ns := String(10)

	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getServicesForNameSpace(s.repo))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)
	req = mux.SetURLVars(req, map[string]string{"namespace": ns})

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var returnedServices []service
	jsonErr := json.Unmarshal(body, &returnedServices)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}
	assert.Equal(t, len([]service{}), len(returnedServices))
}
func TestGetAllChecksForServiceReturnsEmptyListWhenNoneExist(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()
	svc := String(10)

	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getAllChecksForService(s.repo))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)
	req = mux.SetURLVars(req, map[string]string{"service": svc})

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var returnedChecks []healthcheckResp
	jsonErr := json.Unmarshal(body, &returnedChecks)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}
	assert.Equal(t, len([]healthcheckResp{}), len(returnedChecks))
}
func TestGetAllChecksForService(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()

	// Generate some service and namespace names
	ns1Name := String(10)
	ns2Name := String(10)
	s1Name := String(10)
	s2Name := String(10)

	// Create checks for a single service in a specific namespace
	chk1 := generateDummyCheck(s1Name, ns1Name)
	chk2 := generateDummyCheck(s1Name, ns1Name)

	// Create a check against a different service in the same namespace
	chk3 := generateDummyCheck(s2Name, ns1Name)

	// Create a check against a service with the same name, but within a different namespace
	chk4 := generateDummyCheck(s1Name, ns2Name)
	insertItems(s.repo, chk1, chk2, chk3, chk4)

	// We only expect checks returned for a specific service within a specific namespace
	expectedHealthChecks := []healthcheckResp{chk1, chk2}

	// Make the request
	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getAllChecksForService(s.repo))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)
	req = mux.SetURLVars(req, map[string]string{"namespace": ns1Name, "service": s1Name})

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	// Get the returned health check response
	var returnedHealthChecks []healthcheckResp
	jsonErr := json.Unmarshal(body, &returnedHealthChecks)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}

	// Check we have the number of checks for the service that belongs to the correct namespace (ns1Name)
	require.Equal(t, len(expectedHealthChecks), len(returnedHealthChecks))

	for _, expHC := range expectedHealthChecks {
		returnedHealthCheck := findHealthcheckRespByError(expHC.Error, returnedHealthChecks)
		assert.Equal(t, expHC.Error, returnedHealthCheck.Error)
		assert.Equal(t, expHC.CheckTime.Format("2006-01-02T15:04:05.000Z"), returnedHealthCheck.CheckTime.Format("2006-01-02T15:04:05.000Z")) // Formatted since mongo returns timestamps with truncated accuracy
		assert.Equal(t, expHC.StatusCode, returnedHealthCheck.StatusCode)
		assert.Equal(t, expHC.Service.Name, returnedHealthCheck.Service.Name)
		assert.Equal(t, expHC.Service.Namespace, returnedHealthCheck.Service.Namespace)
		assert.Equal(t, expHC.Service.HealthcheckURL, returnedHealthCheck.Service.HealthcheckURL)
		assert.Equal(t, expHC.Service.HealthAnnotations.Port, returnedHealthCheck.Service.HealthAnnotations.Port)
		assert.Equal(t, expHC.Service.HealthAnnotations.EnableScrape, returnedHealthCheck.Service.HealthAnnotations.EnableScrape)
		assert.Equal(t, expHC.Body.Name, returnedHealthCheck.Body.Name)
		assert.Equal(t, expHC.Body.Health, returnedHealthCheck.Body.Health)
		assert.Equal(t, expHC.Body.Description, returnedHealthCheck.Body.Description)

		for _, expC := range expHC.Body.Checks {
			returnedCheck := findCheckByName(expC.Name, returnedHealthCheck.Body.Checks)
			assert.Equal(t, expC.Action, returnedCheck.Action)
			assert.Equal(t, expC.Health, returnedCheck.Health)
			assert.Equal(t, expC.Impact, returnedCheck.Impact)
			assert.Equal(t, expC.Name, returnedCheck.Name)
			assert.Equal(t, expC.Output, returnedCheck.Output)
		}
	}
}

func TestGetLatestChecksForNamespaceReturnsEmptyListWhenNoneExist(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()
	ns := String(10)

	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getLatestChecksForNamespace(s.repo))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)
	req = mux.SetURLVars(req, map[string]string{"namespace": ns})

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	var returnedChecks []healthcheckResp
	jsonErr := json.Unmarshal(body, &returnedChecks)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}
	assert.Equal(t, len([]healthcheckResp{}), len(returnedChecks))
}

func TestGetLatestChecksForNamespace(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()

	// Generate some service and namespace names
	ns1Name := String(10)
	ns2Name := String(10)
	s1Name := String(10)
	s2Name := String(10)

	// Create checks for a single service in a specific namespace
	chk1 := generateDummyCheck(s1Name, ns1Name)
	time.Sleep(time.Millisecond * 200)
	chk2 := generateDummyCheck(s1Name, ns1Name)

	// Create checks against a different service in the same namespace
	chk3 := generateDummyCheck(s2Name, ns1Name)
	time.Sleep(time.Millisecond * 200)
	chk4 := generateDummyCheck(s2Name, ns1Name)

	// Create a check against a service with the same name, but within a different namespace
	chk5 := generateDummyCheck(s1Name, ns2Name)
	insertItems(s.repo, chk1, chk2, chk3, chk4, chk5)

	// We only expect the LATEST checks to be returned, and only the ones for this namespace
	expectedHealthChecks := []healthcheckResp{chk2, chk4}

	// Make the request
	resp := httptest.NewRecorder()
	handler := http.HandlerFunc(getLatestChecksForNamespace(s.repo))

	req, reqErr := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, reqErr)
	req = mux.SetURLVars(req, map[string]string{"namespace": ns1Name})

	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Result().StatusCode)

	body, readErr := ioutil.ReadAll(resp.Body)
	require.NoError(t, readErr)

	// Get the returned health check response
	var returnedHealthChecks []healthcheckResp
	jsonErr := json.Unmarshal(body, &returnedHealthChecks)
	if jsonErr != nil {
		require.NoError(t, jsonErr)
	}

	// Check we have the number of checks for the services that belongs to the correct namespace (ns1Name)
	require.Equal(t, len(expectedHealthChecks), len(returnedHealthChecks))

	for _, expHC := range expectedHealthChecks {
		returnedHealthCheck := findHealthcheckRespByError(expHC.Error, returnedHealthChecks)
		assert.Equal(t, expHC.Error, returnedHealthCheck.Error)
		assert.Equal(t, expHC.CheckTime.Format("2006-01-02T15:04:05.000Z"), returnedHealthCheck.CheckTime.Format("2006-01-02T15:04:05.000Z")) // Formatted since mongo returns timestamps with truncated accuracy
		assert.Equal(t, expHC.StatusCode, returnedHealthCheck.StatusCode)
		assert.Equal(t, expHC.Service.Name, returnedHealthCheck.Service.Name)
		assert.Equal(t, expHC.Service.Namespace, returnedHealthCheck.Service.Namespace)
		assert.Equal(t, expHC.Service.HealthcheckURL, returnedHealthCheck.Service.HealthcheckURL)
		assert.Equal(t, expHC.Service.HealthAnnotations.Port, returnedHealthCheck.Service.HealthAnnotations.Port)
		assert.Equal(t, expHC.Service.HealthAnnotations.EnableScrape, returnedHealthCheck.Service.HealthAnnotations.EnableScrape)
		assert.Equal(t, expHC.Body.Name, returnedHealthCheck.Body.Name)
		assert.Equal(t, expHC.Body.Health, returnedHealthCheck.Body.Health)
		assert.Equal(t, expHC.Body.Description, returnedHealthCheck.Body.Description)

		for _, expC := range expHC.Body.Checks {
			returnedCheck := findCheckByName(expC.Name, returnedHealthCheck.Body.Checks)
			assert.Equal(t, expC.Action, returnedCheck.Action)
			assert.Equal(t, expC.Health, returnedCheck.Health)
			assert.Equal(t, expC.Impact, returnedCheck.Impact)
			assert.Equal(t, expC.Name, returnedCheck.Name)
			assert.Equal(t, expC.Output, returnedCheck.Output)
		}
	}
}

func createNamespace() namespace {
	return namespace{
		Name: String(10),
		HealthAnnotations: healthAnnotations{
			Port:         "8080",
			EnableScrape: "true",
		},
	}
}

const charset = "abcdefghijklmnopqrstuvwxyz" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var seededRand *rand.Rand = rand.New(
	rand.NewSource(time.Now().UnixNano()))

func StringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func String(length int) string {
	return StringWithCharset(length, charset)
}

func generateDummyCheck(serviceName string, namespaceName string, state ...string) healthcheckResp {
	var healthCheck healthcheckResp

	var svc service
	svc.Name = serviceName
	svc.Namespace = namespaceName
	svc.HealthcheckURL = fmt.Sprintf("http://%s.%s/__/health", namespaceName, serviceName)
	svc.HealthAnnotations.EnableScrape = "true"
	svc.HealthAnnotations.Port = "3000"
	healthCheck.Service = svc

	var checkBody healthcheckBody

	var health string
	if len(state) > 0 {
		health = state[0]
	} else {
		health = randomHealthState()
	}

	checkBody.Name = "Check Name " + String(10)
	checkBody.Description = "Check Description " + String(10)
	checkBody.Health = health

	var checks []check
	for i := 0; i < 3; i++ {
		chk := check{
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

func insertItems(mgoRepo *MongoRepository, objs ...interface{}) {
	for _, obj := range objs {
		insertItem(mgoRepo, obj)
	}
}

func insertItem(mgoRepo *MongoRepository, obj interface{}) {
	var objType string
	var collection string
	switch v := obj.(type) {
	case service:
		obj = obj.(service)
		objType = fmt.Sprintf("%T", v)
		collection = servicesCollection
	case namespace:
		obj = obj.(namespace)
		objType = fmt.Sprintf("%T", v)
		collection = namespacesCollection
	case healthcheckResp:
		obj = obj.(healthcheckResp)
		objType = fmt.Sprintf("%T", v)
		collection = healthchecksCollection
	default:
		log.Fatalf("Unknown object: %T", v)
	}

	repoCopy := mgoRepo.WithNewSession()
	defer repoCopy.Close()

	col := repoCopy.db().C(collection)

	err := col.Insert(obj)
	if err != nil {
		log.WithError(err).Errorf("failed to insert %v", objType)
		return
	}
}

func findHealthcheckRespByError(searchText string, hList []healthcheckResp) healthcheckResp {

	for _, h := range hList {
		if h.Error == searchText {
			return h
		}
	}
	return healthcheckResp{}
}

func findCheckByName(searchText string, cList []check) check {
	var chk check
	for _, c := range cList {
		if c.Name == searchText {
			return c
		}
	}
	return chk
}

func findNamespaceByName(searchNS namespace, nsList []namespace) namespace {

	for _, ns := range nsList {
		if ns.Name == searchNS.Name {
			return ns
		}
	}
	return namespace{}
}

func findServiceByName(searchS service, sList []service) service {

	for _, s := range sList {
		if s.Name == searchS.Name {
			return s
		}
	}
	return service{}
}

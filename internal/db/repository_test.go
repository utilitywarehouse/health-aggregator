package db

import (
	"math/rand"
	"strconv"
	"time"

	"github.com/stretchr/testify/require"

	"fmt"

	"testing"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/pborman/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/utilitywarehouse/health-aggregator/internal/constants"
	"github.com/utilitywarehouse/health-aggregator/internal/helpers"
	"github.com/utilitywarehouse/health-aggregator/internal/instrumentation"
	"github.com/utilitywarehouse/health-aggregator/internal/model"
)

var (
	defaultHealthAnnotations  = model.HealthAnnotations{EnableScrape: "true", Port: "8081"}
	diffPortHealthAnnotations = model.HealthAnnotations{EnableScrape: "true", Port: "8080"}
	noScrapeHealthAnnotations = model.HealthAnnotations{EnableScrape: "false", Port: "8081"}
)

const (
	dbURL = "localhost:27017"
)

type TestSuite struct {
	repo    *MongoRepository
	session *mgo.Session
	dbName  string
}

var s TestSuite

func (s *TestSuite) SetUpTest() {
	sess, err := mgo.Dial(dbURL)
	if err != nil {
		log.Fatalf("failed to create mongo session: %s", err.Error())
	}
	s.session = sess
	s.dbName = uuid.New()
	s.repo = NewMongoRepository(s.session, s.dbName)
}

func (s *TestSuite) TearDownTest() {
	s.session.DB(s.dbName).DropDatabase()
	s.repo.Close()
}

func Test_GetHealthchecks(t *testing.T) {
	s.SetUpTest()
	defer s.TearDownTest()

	metrics := instrumentation.SetupMetrics()

	// Create Services with Health Annotations config
	ns1Name := helpers.String(10)
	ns2Name := helpers.String(10)

	s1 := helpers.GenerateDummyServiceForNamespace(ns1Name, 1)
	s1.HealthAnnotations = defaultHealthAnnotations

	s2 := helpers.GenerateDummyServiceForNamespace(ns1Name, 1)
	s2.HealthAnnotations = noScrapeHealthAnnotations

	s3 := helpers.GenerateDummyServiceForNamespace(ns2Name, 1)
	s3.HealthAnnotations = diffPortHealthAnnotations

	// this service has no replicas and should not be checked
	s4 := helpers.GenerateDummyServiceForNamespace(ns1Name, 0)
	s4.HealthAnnotations = defaultHealthAnnotations

	insertItems(s.repo, s1, s2, s3)

	errsChan := make(chan error, 10)
	healthchecksNS1 := make(chan model.Service, 10)
	healthchecksNS2 := make(chan model.Service, 10)
	healthchecksAll := make(chan model.Service, 10)

	// Restricted to Namespace 1
	expectedServicesNS1 := []model.Service{s1}

	// Restricted to Namespace 2
	expectedServicesNS2 := []model.Service{s3}

	// Unrestricted
	expectedServicesAll := []model.Service{s1, s3}

	done := make(chan struct{})
	go func() {
		GetHealthchecks(s.repo, healthchecksNS1, errsChan, metrics, ns1Name)
		close(healthchecksNS1)
		GetHealthchecks(s.repo, healthchecksNS2, errsChan, metrics, ns2Name)
		close(healthchecksNS2)
		GetHealthchecks(s.repo, healthchecksAll, errsChan, metrics)
		close(healthchecksAll)
		close(done)
	}()

	select {
	case <-errsChan:
		t.Errorf("Should not get an error")
	default:
	}

	<-done

	var returnedServices []model.Service

	for check := range healthchecksNS1 {
		returnedServices = append(returnedServices, check)
	}

	assert.NoError(t, helpers.TestSliceServicesEquality(expectedServicesNS1, returnedServices))
	returnedServices = returnedServices[:0]

	for check := range healthchecksNS2 {
		returnedServices = append(returnedServices, check)
	}
	assert.NoError(t, helpers.TestSliceServicesEquality(expectedServicesNS2, returnedServices))
	returnedServices = returnedServices[:0]

	for check := range healthchecksAll {
		returnedServices = append(returnedServices, check)
	}
	assert.NoError(t, helpers.TestSliceServicesEquality(expectedServicesAll, returnedServices))

	close(errsChan)
}
func Test_FindAllServicesWithHealthScrapeEnabled(t *testing.T) {
	s.SetUpTest()
	defer s.TearDownTest()

	// Create Services with Health Annotations config
	ns1Name := helpers.String(10)
	ns2Name := helpers.String(10)
	ns3Name := helpers.String(10)
	ns4Name := helpers.String(10)

	s1 := helpers.GenerateDummyServiceForNamespace(ns1Name, 1)
	s1.HealthAnnotations = defaultHealthAnnotations

	s2 := helpers.GenerateDummyServiceForNamespace(ns1Name, 1)
	s2.HealthAnnotations = noScrapeHealthAnnotations

	s3 := helpers.GenerateDummyServiceForNamespace(ns2Name, 1)
	s3.HealthAnnotations = diffPortHealthAnnotations

	s4 := helpers.GenerateDummyServiceForNamespace(ns3Name, 1)
	s4.HealthAnnotations = defaultHealthAnnotations

	s5 := helpers.GenerateDummyServiceForNamespace(ns4Name, 1)
	s5.HealthAnnotations = noScrapeHealthAnnotations

	insertItems(s.repo, s1, s2, s3, s4, s5)

	// Restricted to Namespace 1
	expectedServicesNS1 := []model.Service{s1}

	// Restricted to Namespace 2
	expectedServicesNS2 := []model.Service{s3}

	// Restricted to Namespace 1 and 3
	expectedServicesNS1NS3 := []model.Service{s1, s4}

	// Restricted to Namespace 4
	expectedServicesNS4 := []model.Service{}

	// Unrestricted
	expectedServicesAll := []model.Service{s1, s3, s4}

	returnedServices, err := FindAllServicesWithHealthScrapeEnabled(s.repo, ns1Name)
	require.NoError(t, err)
	assert.NoError(t, helpers.TestSliceServicesEquality(expectedServicesNS1, returnedServices))

	returnedServices, err = FindAllServicesWithHealthScrapeEnabled(s.repo, ns2Name)
	require.NoError(t, err)
	assert.NoError(t, helpers.TestSliceServicesEquality(expectedServicesNS2, returnedServices))

	returnedServices, err = FindAllServicesWithHealthScrapeEnabled(s.repo, ns1Name, ns3Name)
	require.NoError(t, err)
	assert.NoError(t, helpers.TestSliceServicesEquality(expectedServicesNS1NS3, returnedServices))

	returnedServices, err = FindAllServicesWithHealthScrapeEnabled(s.repo, ns4Name)
	require.NoError(t, err)
	assert.NoError(t, helpers.TestSliceServicesEquality(expectedServicesNS4, returnedServices))

	returnedServices, err = FindAllServicesWithHealthScrapeEnabled(s.repo)
	require.NoError(t, err)
	assert.NoError(t, helpers.TestSliceServicesEquality(expectedServicesAll, returnedServices))

	allNamespaces := []string{}
	returnedServices, err = FindAllServicesWithHealthScrapeEnabled(s.repo, allNamespaces...)
	require.NoError(t, err)
	assert.NoError(t, helpers.TestSliceServicesEquality(expectedServicesAll, returnedServices))
}

func Test_FindAllServices(t *testing.T) {
	s.SetUpTest()
	defer s.TearDownTest()

	s1 := generateDummyService(helpers.String(10))
	s2 := generateDummyService(helpers.String(10))

	expectedServices := []model.Service{s1, s2}

	insertItems(s.repo, s1, s2)

	returnedServices, err := FindAllServices(s.repo)
	require.NoError(t, err)
	assert.NoError(t, helpers.TestSliceServicesEquality(expectedServices, returnedServices))
}

func Test_FindAllNamespaces(t *testing.T) {
	s.SetUpTest()
	defer s.TearDownTest()

	n1 := generateDummyNamespace()
	n2 := generateDummyNamespace()

	expectedNamespaces := []model.Namespace{n1, n2}

	insertItems(s.repo, n1, n2)

	returnedNamespaces, err := FindAllNamespaces(s.repo)
	require.NoError(t, err)
	assert.NoError(t, helpers.TestSliceNamespacesEquality(expectedNamespaces, returnedNamespaces))
}

func Test_FindAllNamespacesReturnsEmptySliceWhenNoneExist(t *testing.T) {
	s.SetUpTest()
	defer s.TearDownTest()

	expectedNamespaces := []model.Namespace{}

	returnedNamespaces, err := FindAllNamespaces(s.repo)
	require.NoError(t, err)
	assert.NoError(t, helpers.TestSliceNamespacesEquality(expectedNamespaces, returnedNamespaces))
}

func Test_FindAllServicesForNamespace(t *testing.T) {
	s.SetUpTest()
	defer s.TearDownTest()

	ns1Name := helpers.String(10)
	ns2Name := helpers.String(10)

	s1 := generateDummyService(ns1Name)
	s2 := generateDummyService(ns1Name)
	s3 := generateDummyService(ns2Name)

	insertItems(s.repo, s1, s2, s3)

	expectedServicesForNS1 := []model.Service{s1, s2}
	expectedServicesForNS2 := []model.Service{s3}

	returnedServicesForNS1, err := FindAllServicesForNamespace(s.repo, ns1Name)
	require.NoError(t, err)
	assert.NoError(t, helpers.TestSliceServicesEquality(expectedServicesForNS1, returnedServicesForNS1))

	returnedServicesForNS2, err := FindAllServicesForNamespace(s.repo, ns2Name)
	require.NoError(t, err)
	assert.NoError(t, helpers.TestSliceServicesEquality(expectedServicesForNS2, returnedServicesForNS2))
}

func Test_FindAllServicesForNamespaceReturnsEmptySliceWhenNoneExist(t *testing.T) {
	s.SetUpTest()
	defer s.TearDownTest()

	expectedServices := []model.Service{}

	returnedServicesForNonExistentNamespace, err := FindAllServicesForNamespace(s.repo, "madeUpNamespace")
	require.NoError(t, err)
	assert.NoError(t, helpers.TestSliceServicesEquality(expectedServices, returnedServicesForNonExistentNamespace))
}

func Test_UpsertServiceConfigs(t *testing.T) {
	s.SetUpTest()
	defer s.TearDownTest()

	// Create test data
	ns1Name := helpers.String(10)
	s1Name := helpers.String(10)
	s2Name := helpers.String(10)

	// Create services
	s1 := model.Service{
		Name:              s1Name,
		Namespace:         ns1Name,
		HealthAnnotations: defaultHealthAnnotations,
		Deployment: model.Deployment{
			DesiredReplicas: 1,
		},
		AppPort: "80",
	}

	s2 := model.Service{
		Name:              s2Name,
		Namespace:         ns1Name,
		HealthAnnotations: noScrapeHealthAnnotations,
		Deployment: model.Deployment{
			DesiredReplicas: 1,
		},
		AppPort: "80",
	}

	// Persist services
	insertItems(s.repo, s1, s2)

	servicesChan := make(chan model.Service, 10)

	k := NewK8sServicesConfigUpdater(servicesChan, s.repo.WithNewSession())

	// Prepare Upsert to run in the background and signal when finished
	done := make(chan struct{})
	go func() {
		k.UpsertServiceConfigs()
		close(done)
	}()

	// Updated service data
	updatedS1 := model.Service{
		Name:              s1Name,
		Namespace:         ns1Name,
		HealthAnnotations: model.HealthAnnotations{EnableScrape: "false", Port: "3000"},
		Deployment: model.Deployment{
			DesiredReplicas: 2,
		},
		AppPort: "8080",
	}

	updatedS2 := model.Service{
		Name:              s2Name,
		Namespace:         ns1Name,
		HealthAnnotations: model.HealthAnnotations{EnableScrape: "false", Port: "3000"},
		Deployment: model.Deployment{
			DesiredReplicas: 3,
		},
		AppPort: "8090",
	}

	// Push services for processing
	servicesChan <- updatedS1
	servicesChan <- updatedS2
	close(servicesChan)

	// Wait for signal to show upsert complete
	<-done

	svc1 := findService(s1Name, ns1Name, s.repo)
	svc2 := findService(s2Name, ns1Name, s.repo)

	// Service 1 asserts
	assert.Equal(t, updatedS1.HealthAnnotations.EnableScrape, svc1.HealthAnnotations.EnableScrape)
	assert.Equal(t, updatedS1.HealthAnnotations.Port, svc1.HealthAnnotations.Port)
	assert.Equal(t, updatedS1.Deployment.DesiredReplicas, svc1.Deployment.DesiredReplicas)
	assert.Equal(t, updatedS1.AppPort, svc1.AppPort)

	// Service 2 asserts
	assert.Equal(t, updatedS2.HealthAnnotations.EnableScrape, svc2.HealthAnnotations.EnableScrape)
	assert.Equal(t, updatedS2.HealthAnnotations.Port, svc2.HealthAnnotations.Port)
	assert.Equal(t, updatedS2.Deployment.DesiredReplicas, svc2.Deployment.DesiredReplicas)
	assert.Equal(t, updatedS2.AppPort, svc2.AppPort)
}

func Test_UpsertNamespaceConfigs(t *testing.T) {
	s.SetUpTest()
	defer s.TearDownTest()

	// Create test data
	ns1Name := helpers.String(10)
	ns2Name := helpers.String(10)

	// Create namespaces
	n1 := model.Namespace{
		Name:              ns1Name,
		HealthAnnotations: defaultHealthAnnotations,
	}

	n2 := model.Namespace{
		Name:              ns2Name,
		HealthAnnotations: defaultHealthAnnotations,
	}

	// Persist namespaces
	insertItems(s.repo, n1, n2)

	namespacesChan := make(chan model.Namespace, 10)

	// Updated service data
	updatedNS1 := model.Namespace{
		Name:              ns1Name,
		HealthAnnotations: model.HealthAnnotations{EnableScrape: "false", Port: "3000"},
	}

	updatedNS2 := model.Namespace{
		Name:              ns2Name,
		HealthAnnotations: model.HealthAnnotations{EnableScrape: "false", Port: "8090"},
	}

	// Push services for processing
	namespacesChan <- updatedNS1
	namespacesChan <- updatedNS2
	close(namespacesChan)

	k := NewK8sNamespacesConfigUpdater(namespacesChan, s.repo.WithNewSession())
	k.UpsertNamespaceConfigs()

	ns1 := findNamespace(ns1Name, s.repo)
	ns2 := findNamespace(ns2Name, s.repo)

	// Service 1 asserts
	assert.Equal(t, updatedNS1.HealthAnnotations.EnableScrape, ns1.HealthAnnotations.EnableScrape)
	assert.Equal(t, updatedNS1.HealthAnnotations.Port, ns1.HealthAnnotations.Port)

	// Service 2 asserts
	assert.Equal(t, updatedNS2.HealthAnnotations.EnableScrape, ns2.HealthAnnotations.EnableScrape)
	assert.Equal(t, updatedNS2.HealthAnnotations.Port, ns2.HealthAnnotations.Port)
}

func Test_InsertHealthcheckResponses(t *testing.T) {
	s.SetUpTest()
	defer s.TearDownTest()

	s1Name := helpers.String(10)
	ns1Name := helpers.String(10)
	ns2Name := helpers.String(10)
	podNames := []string{"pod-a", "pod-b"}

	check1 := helpers.GenerateDummyServiceStatus(s1Name, ns1Name, podNames)
	check2 := helpers.GenerateDummyServiceStatus(s1Name, ns1Name, podNames)
	check3 := helpers.GenerateDummyServiceStatus(s1Name, ns2Name, podNames)

	servicesChan := make(chan model.ServiceStatus, 10)
	errsChan := make(chan error, 10)

	servicesChan <- check1
	servicesChan <- check2
	servicesChan <- check3
	close(servicesChan)

	done := make(chan struct{})
	go func() {
		InsertHealthcheckResponses(s.repo, servicesChan, errsChan)
		close(done)
	}()

	select {
	case <-errsChan:
		t.Errorf("Should not get an error")
	default:
	}

	<-done

	retrievedChecks := findAllServiceStatuses(s.repo)
	assert.NoError(t, helpers.TestServiceStatusesEquality([]model.ServiceStatus{check1, check2, check3}, retrievedChecks))

	close(errsChan)
}

func Test_RemoveChecksOlderThan(t *testing.T) {
	s.SetUpTest()
	defer s.TearDownTest()

	sName := helpers.String(10)
	nsName := helpers.String(10)
	podNames := []string{"pod-a", "pod-b"}

	oldCheck := helpers.GenerateDummyServiceStatus(sName, nsName, podNames)
	oldCheck.CheckTime = time.Now().Add(time.Hour * -25).UTC()

	newCheck := helpers.GenerateDummyServiceStatus(sName, nsName, podNames)
	newCheck.CheckTime = time.Now().Add(time.Hour * -23).UTC()
	newCheck.Error = "something specific"

	insertItems(s.repo, oldCheck, newCheck)

	errsChan := make(chan error, 10)
	done := make(chan struct{})

	go func() {
		RemoveChecksOlderThan(1, s.repo, errsChan)
		close(done)
	}()

	select {
	case <-errsChan:
		t.Errorf("Should not get an error")
	default:
	}

	<-done
	remainingChecks := findAllServiceStatuses(s.repo)

	require.Equal(t, 1, len(remainingChecks))
	assert.Equal(t, newCheck.Error, remainingChecks[0].Error)

	close(errsChan)
}

func Test_DropDB(t *testing.T) {
	s.SetUpTest()
	defer s.TearDownTest()

	ns := generateDummyNamespace()
	svc := generateDummyService(ns.Name)
	check := helpers.GenerateDummyServiceStatus(svc.Name, ns.Name, []string{"pod-a", "pod-b"})

	insertItems(s.repo, ns, svc, check)

	err := DropDB(s.repo)
	require.NoError(t, err)

	assert.True(t, len(findAllServiceStatuses(s.repo)) == 0)
	assert.True(t, len(findAllNamespaces(s.repo)) == 0)
	assert.True(t, len(findAllServices(s.repo)) == 0)
}

func Test_ProcessDeployment(t *testing.T) {
	s.SetUpTest()
	defer s.TearDownTest()

	repo := s.repo.WithNewSession()

	nsName := helpers.String(10)
	service1 := generateDummyService(nsName)
	service2 := generateDummyService(nsName)
	service3 := generateDummyService(nsName)

	service1.Deployment.DesiredReplicas = 0
	service2.Deployment.DesiredReplicas = 2
	service3.Deployment.DesiredReplicas = 3

	insertItems(s.repo, service1, service2, service3)

	updater := NewUpdaterService(nil, nil, repo)

	service1UpdatedDeployment := model.Deployment{DesiredReplicas: 2, Service: service1.Name, Namespace: nsName}
	service2UpdatedDeployment := model.Deployment{DesiredReplicas: 4, Service: service2.Name, Namespace: nsName}
	service3UpdatedDeployment := model.Deployment{DesiredReplicas: 0, Service: service3.Name, Namespace: nsName}

	updateItem1 := model.UpdateItem{Type: "ADDED", Object: service1UpdatedDeployment}
	updateItem2 := model.UpdateItem{Type: "MODIFIED", Object: service2UpdatedDeployment}
	updateItem3 := model.UpdateItem{Type: "DELETED", Object: service3UpdatedDeployment}

	updater.processDeployment(updateItem1)
	assert.Equal(t, int32(2), findService(service1.Name, nsName, repo).Deployment.DesiredReplicas)

	updater.processDeployment(updateItem2)
	assert.Equal(t, int32(4), findService(service2.Name, nsName, repo).Deployment.DesiredReplicas)

	updater.processDeployment(updateItem3)
	assert.Equal(t, int32(0), findService(service3.Name, nsName, repo).Deployment.DesiredReplicas)
}

func Test_DoUpdates(t *testing.T) {
	s.SetUpTest()
	defer s.TearDownTest()

	repo := s.repo.WithNewSession()

	nsName := helpers.String(10)
	service1 := generateDummyService(nsName)
	service2 := generateDummyService(nsName)
	service3 := generateDummyService(nsName)

	service1.Deployment.DesiredReplicas = 1
	service2.Deployment.DesiredReplicas = 1
	service3.Deployment.DesiredReplicas = 1

	insertItems(s.repo, service1, service2, service3)

	updateItems := make(chan model.UpdateItem, 10)
	errs := make(chan error, 10)
	updater := NewUpdaterService(updateItems, errs, repo)

	service1UpdatedDeployment := model.Deployment{DesiredReplicas: 2, Service: service1.Name, Namespace: nsName}
	service2UpdatedDeployment := model.Deployment{DesiredReplicas: 4, Service: service2.Name, Namespace: nsName}
	service3UpdatedDeployment := model.Deployment{DesiredReplicas: 0, Service: service3.Name, Namespace: nsName}

	updateItem1 := model.UpdateItem{Type: "ADDED", Object: service1UpdatedDeployment}
	updateItem2 := model.UpdateItem{Type: "MODIFIED", Object: service2UpdatedDeployment}
	updateItem3 := model.UpdateItem{Type: "DELETED", Object: service3UpdatedDeployment}

	updateItems <- updateItem1
	updateItems <- updateItem2
	updateItems <- updateItem3
	close(updateItems)

	done := make(chan struct{})
	go func() {
		updater.DoUpdates()
		close(done)
	}()

	select {
	case <-errs:
		t.Errorf("Should not get an error")
	default:
	}

	<-done

	assert.Equal(t, int32(2), findService(service1.Name, nsName, repo).Deployment.DesiredReplicas)
	assert.Equal(t, int32(4), findService(service2.Name, nsName, repo).Deployment.DesiredReplicas)
	assert.Equal(t, int32(0), findService(service3.Name, nsName, repo).Deployment.DesiredReplicas)
}

func Test_DoUpdatesUnsupportedObject(t *testing.T) {
	s.SetUpTest()
	defer s.TearDownTest()

	repo := s.repo.WithNewSession()

	updateItems := make(chan model.UpdateItem, 10)
	errs := make(chan error, 10)
	updater := NewUpdaterService(updateItems, errs, repo)

	var emptyStruct struct{}

	updateItem := model.UpdateItem{Type: "ADDED", Object: emptyStruct}

	updateItems <- updateItem
	close(updateItems)

	done := make(chan struct{})
	go func() {
		updater.DoUpdates()
		close(done)
		close(errs)
	}()

	for err := range errs {
		assert.Contains(t, err.Error(), "unsupported")
	}
	<-done
}

func findNamespace(name string, repo *MongoRepository) model.Namespace {
	var n model.Namespace
	s.repo.Db().C(constants.NamespacesCollection).Find(bson.M{"name": name}).One(&n)
	return n
}

func findAllNamespaces(repo *MongoRepository) []model.Namespace {
	var nsList []model.Namespace
	s.repo.Db().C(constants.NamespacesCollection).Find(bson.M{}).All(&nsList)
	return nsList
}

func findService(name string, namespace string, repo *MongoRepository) model.Service {
	var svc model.Service
	s.repo.Db().C(constants.ServicesCollection).Find(bson.M{"namespace": namespace, "name": name}).One(&svc)
	return svc
}

func findAllServices(repo *MongoRepository) []model.Service {
	var sList []model.Service
	s.repo.Db().C(constants.ServicesCollection).Find(bson.M{}).All(&sList)
	return sList
}

func findAllServiceStatuses(repo *MongoRepository) []model.ServiceStatus {
	var checks []model.ServiceStatus
	collection := repo.Db().C(constants.HealthchecksCollection)
	collection.Find(bson.M{}).All(&checks)

	return checks
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
	case model.Service:
		obj = obj.(model.Service)
		objType = fmt.Sprintf("%T", v)
		collection = constants.ServicesCollection
	case model.Namespace:
		obj = obj.(model.Namespace)
		objType = fmt.Sprintf("%T", v)
		collection = constants.NamespacesCollection
	case model.ServiceStatus:
		obj = obj.(model.ServiceStatus)
		objType = fmt.Sprintf("%T", v)
		collection = constants.HealthchecksCollection
	default:
		log.Fatalf("Unknown object: %T", v)
	}

	repoCopy := mgoRepo.WithNewSession()
	defer repoCopy.Close()

	col := repoCopy.Db().C(collection)

	err := col.Insert(obj)
	if err != nil {
		log.WithError(err).Errorf("failed to insert %v", objType)
		return
	}
}

func generateDummyService(namespace string) model.Service {

	randInRange := func(min, max int) int {
		rand.Seed(time.Now().Unix())
		return rand.Intn(max-min) + min
	}

	var svc model.Service
	svc.Name = helpers.String(10)
	svc.Namespace = namespace
	svc.AppPort = strconv.Itoa(randInRange(8080, 9080))
	svc.Deployment = model.Deployment{DesiredReplicas: int32(randInRange(1, 6))}
	svc.HealthAnnotations = model.HealthAnnotations{EnableScrape: "true", Port: strconv.Itoa(randInRange(8080, 9080))}

	return svc
}

func generateDummyNamespace() model.Namespace {

	randInRange := func(min, max int) int {
		rand.Seed(time.Now().Unix())
		return rand.Intn(max-min) + min
	}

	var n model.Namespace
	n.Name = helpers.String(10)
	n.HealthAnnotations = model.HealthAnnotations{EnableScrape: "true", Port: strconv.Itoa(randInRange(8080, 9080))}

	return n
}

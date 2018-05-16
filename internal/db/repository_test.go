package db

import (
	"github.com/stretchr/testify/require"
	//"io/ioutil"
	"fmt"
	//"net/http"

	//"strings"
	"testing"

	"github.com/globalsign/mgo"
	"github.com/pborman/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/utilitywarehouse/health-aggregator/internal/constants"
	"github.com/utilitywarehouse/health-aggregator/internal/helpers"
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

	// Create Services with Health Annotations config
	ns1Name := helpers.String(10)
	ns2Name := helpers.String(10)

	s1 := model.Service{Name: helpers.String(10),
		Namespace:         ns1Name,
		HealthAnnotations: defaultHealthAnnotations,
	}

	s2 := model.Service{Name: helpers.String(10),
		Namespace:         ns1Name,
		HealthAnnotations: noScrapeHealthAnnotations,
	}

	s3 := model.Service{Name: helpers.String(10),
		Namespace:         ns2Name,
		HealthAnnotations: diffPortHealthAnnotations,
	}

	insertItems(s.repo, s1, s2, s3)

	errs := make(chan error, 10)
	healthchecksNS1 := make(chan model.Service, 10)
	healthchecksNS2 := make(chan model.Service, 10)
	healthchecksAll := make(chan model.Service, 10)

	// Restricted to Namespace 1
	expectedServicesNS1 := []model.Service{s1}

	// Restricted to Namespace 2
	expectedServicesNS2 := []model.Service{s3}

	// Unrestricted
	expectedServicesAll := []model.Service{s1, s3}

	go func() {
		GetHealthchecks(s.repo, healthchecksNS1, errs, ns1Name)
		close(healthchecksNS1)
		GetHealthchecks(s.repo, healthchecksNS2, errs, ns2Name)
		close(healthchecksNS2)
		GetHealthchecks(s.repo, healthchecksAll, errs)
		close(healthchecksAll)
		close(errs)
	}()

	go func() {
		for e := range errs {
			t.Errorf("Should not get an error but received %v", e)
		}
	}()

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

}

func Test_FindAllServicesWithHealthScrapeEnabled(t *testing.T) {
	s.SetUpTest()
	defer s.TearDownTest()

	// Create Services with Health Annotations config
	ns1Name := helpers.String(10)
	ns2Name := helpers.String(10)
	ns3Name := helpers.String(10)
	ns4Name := helpers.String(10)

	s1 := model.Service{Name: helpers.String(10),
		Namespace:         ns1Name,
		HealthAnnotations: defaultHealthAnnotations,
	}

	s2 := model.Service{Name: helpers.String(10),
		Namespace:         ns1Name,
		HealthAnnotations: noScrapeHealthAnnotations,
	}

	s3 := model.Service{Name: helpers.String(10),
		Namespace:         ns2Name,
		HealthAnnotations: diffPortHealthAnnotations,
	}

	s4 := model.Service{Name: helpers.String(10),
		Namespace:         ns3Name,
		HealthAnnotations: defaultHealthAnnotations,
	}

	s5 := model.Service{Name: helpers.String(10),
		Namespace:         ns4Name,
		HealthAnnotations: noScrapeHealthAnnotations,
	}

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

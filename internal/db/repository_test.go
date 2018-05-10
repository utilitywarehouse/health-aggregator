package db

import (
	//"io/ioutil"
	"fmt"
	//"net/http"
	"os"
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
	repo *MongoRepository
}

var s TestSuite

func TestMain(m *testing.M) {
	sess, err := mgo.Dial(dbURL)
	if err != nil {
		log.Fatalf("failed to create mongo session: %s", err.Error())
	}
	defer sess.Close()

	s.repo = NewMongoRepository(sess, uuid.New())

	code := m.Run()
	dbErr := s.repo.Session.DB(s.repo.DBName).DropDatabase()
	if dbErr != nil {
		log.Printf("Failed to drop database %v", s.repo.DBName)
	}
	os.Exit(code)
}

func Test_GetHealthchecks(t *testing.T) {
	repoCopy := s.repo.WithNewSession()
	defer repoCopy.Close()

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

	insertItems(repoCopy, s1, s2, s3)

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
		GetHealthchecks(ns1Name, s.repo, healthchecksNS1, errs)
		close(healthchecksNS1)
		GetHealthchecks(ns2Name, s.repo, healthchecksNS2, errs)
		close(healthchecksNS2)
		GetHealthchecks("", s.repo, healthchecksAll, errs)
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
	assert.True(t, helpers.TestSliceServicesEquality(expectedServicesNS1, returnedServices))
	returnedServices = returnedServices[:0]

	for check := range healthchecksNS2 {
		returnedServices = append(returnedServices, check)
	}
	assert.True(t, helpers.TestSliceServicesEquality(expectedServicesNS2, returnedServices))
	returnedServices = returnedServices[:0]

	for check := range healthchecksAll {
		returnedServices = append(returnedServices, check)
	}
	assert.True(t, helpers.TestSliceServicesEquality(expectedServicesAll, returnedServices))

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
	case model.HealthcheckResp:
		obj = obj.(model.HealthcheckResp)
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

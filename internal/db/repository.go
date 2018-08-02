package db

import (
	"fmt"
	"time"

	"github.com/pkg/errors"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	log "github.com/sirupsen/logrus"
	"github.com/utilitywarehouse/health-aggregator/internal/constants"
	"github.com/utilitywarehouse/health-aggregator/internal/model"
)

// K8sServicesConfigUpdater is a receiver object allowing UpsertServiceConfigs to be called
type K8sServicesConfigUpdater struct {
	Services chan model.Service
	Repo     *MongoRepository
}

// K8sNamespacesConfigUpdater is a receiver object allowing UpsertNamespaceConfigs to be called
type K8sNamespacesConfigUpdater struct {
	Namespaces chan model.Namespace
	Repo       *MongoRepository
}

// NewK8sServicesConfigUpdater creates a new K8sServicesConfigUpdater
func NewK8sServicesConfigUpdater(services chan model.Service, repo *MongoRepository) K8sServicesConfigUpdater {

	return K8sServicesConfigUpdater{
		Services: services,
		Repo:     repo,
	}
}

// NewK8sNamespacesConfigUpdater creates a new K8sNamespacesConfigUpdater
func NewK8sNamespacesConfigUpdater(namespaces chan model.Namespace, repo *MongoRepository) K8sNamespacesConfigUpdater {

	return K8sNamespacesConfigUpdater{
		Namespaces: namespaces,
		Repo:       repo,
	}
}

// UpsertServiceConfigs inserts or updates Services from a provided cannel of type Service, sending any
// errors to a channel of type error
func (k K8sServicesConfigUpdater) UpsertServiceConfigs() {

	defer k.Repo.Close()
	for s := range k.Services {
		collection := k.Repo.Db().C(constants.ServicesCollection)

		_, err := collection.Upsert(bson.M{"name": s.Name, "namespace": s.Namespace}, s)
		if err != nil {
			log.WithError(err).Errorf("failed to insert service %s in namespace %s", s.Name, s.Namespace)
			return
		}
	}
}

// UpsertNamespaceConfigs inserts or updates Namespaces from a provided cannel of type Namespace, sending any
// errors to a channel of type error
func (k K8sNamespacesConfigUpdater) UpsertNamespaceConfigs() {

	defer k.Repo.Close()
	for n := range k.Namespaces {

		collection := k.Repo.Db().C(constants.NamespacesCollection)

		_, err := collection.Upsert(bson.M{"name": n.Name}, n)
		if err != nil {
			log.WithError(err).Errorf("failed to insert namespace %s", n.Name)
			return
		}
	}
}

// InsertHealthcheckResponses inserts health check responses picked from a channel of type ServiceStatus, sending any
// errors to a channel of type error
func InsertHealthcheckResponses(mgoRepo *MongoRepository, statusResponses chan model.ServiceStatus, errs chan error) {

	repoCopy := mgoRepo.WithNewSession()
	defer repoCopy.Close()

	for r := range statusResponses {

		collection := repoCopy.Db().C(constants.HealthchecksCollection)

		var prevCheckResponse model.ServiceStatus
		if err := collection.Find(bson.M{"service.namespace": r.Service.Namespace, "service.name": r.Service.Name}).Sort("-checkTime").Limit(1).One(&prevCheckResponse); err != nil {
			if err != mgo.ErrNotFound {
				log.WithError(err).Errorf("failed to get previous healthcheck response for %s for namespace %s", r.Service.Name, r.Service.Namespace)
			}
		}

		if prevCheckResponse.AggregatedState != r.AggregatedState {
			r.StateSince = r.CheckTime
			r.PreviousState = prevCheckResponse.AggregatedState
		} else {
			r.StateSince = prevCheckResponse.StateSince
			r.PreviousState = prevCheckResponse.PreviousState
		}

		err := collection.Insert(r)
		if err != nil {
			log.WithError(err).Errorf("failed to insert healthcheck response for %s for namespace %s", r.Service.Name, r.Service.Namespace)
			return
		}
	}
}

// FindAllServices finds all Services regardless of Namespace
func FindAllServices(mgoRepo *MongoRepository) ([]model.Service, error) {

	collection := mgoRepo.Db().C(constants.ServicesCollection)

	var services []model.Service
	if err := collection.Find(bson.M{}).All(&services); err != nil {
		return nil, errors.New("failed to get all services")
	}

	return services, nil
}

// FindAllNamespaces finds all Namespaces
func FindAllNamespaces(mgoRepo *MongoRepository) ([]model.Namespace, error) {

	collection := mgoRepo.Db().C(constants.NamespacesCollection)

	var ns []model.Namespace
	if err := collection.Find(bson.M{}).All(&ns); err != nil {
		return nil, errors.New("failed to get all namespaces")
	}

	if ns == nil {
		ns = []model.Namespace{}
	}

	return ns, nil
}

// FindAllServicesForNamespace finds all Services for a given Namespace Name
func FindAllServicesForNamespace(mgoRepo *MongoRepository, ns string) ([]model.Service, error) {

	collection := mgoRepo.Db().C(constants.ServicesCollection)

	var svcs []model.Service
	if err := collection.Find(bson.M{"namespace": ns}).All(&svcs); err != nil {
		return nil, fmt.Errorf("failed to get all services for namespace %s", ns)
	}

	if svcs == nil {
		svcs = []model.Service{}
	}

	return svcs, nil
}

// FindAllServicesWithHealthScrapeEnabled finds all Services where the EnableScrape from HealthAnnotations is true
func FindAllServicesWithHealthScrapeEnabled(mgoRepo *MongoRepository, restrictToNamespace ...string) ([]model.Service, error) {

	collection := mgoRepo.Db().C(constants.ServicesCollection)
	svcs := []model.Service{}
	if len(restrictToNamespace) > 0 {
		if err := collection.Find(bson.M{"namespace": bson.M{"$in": restrictToNamespace}, "healthAnnotations.enableScrape": "true", "deployment.desiredReplicas": bson.M{"$gt": 0}}).Sort("namespace").All(&svcs); err != nil {
			return nil, errors.Wrap(err, "failed to get all service healthcheck endpoints with scrape enabled")
		}
		return svcs, nil
	}
	if err := collection.Find(bson.M{"healthAnnotations.enableScrape": "true"}).Sort("namespace").All(&svcs); err != nil {
		return nil, errors.Wrap(err, "failed to get all service healthcheck endpoints with scrape enabled")
	}

	return svcs, nil
}

// FindAllChecksForService returns the last 50 ServiceStatus for a given Service and Namespace string in CheckTime
// descending order
func FindAllChecksForService(mgoRepo *MongoRepository, n string, s string) ([]model.ServiceStatus, error) {

	collection := mgoRepo.Db().C(constants.HealthchecksCollection)

	var checks []model.ServiceStatus
	if err := collection.Find(bson.M{"service.namespace": n, "service.name": s}).Limit(50).Sort("-checkTime").All(&checks); err != nil {
		return nil, fmt.Errorf("failed to get all healthcheck responses for service %v in namespace %v", s, n)
	}

	if checks == nil {
		checks = []model.ServiceStatus{}
	}

	return checks, nil
}

// FindLatestChecksForNamespace returns the latest ServiceStatus for all services in a given Namespace Name
func FindLatestChecksForNamespace(mgoRepo *MongoRepository, n string) ([]model.ServiceStatus, error) {

	var servicesToReturn []model.Service
	servicesToReturn, err := FindAllServicesWithHealthScrapeEnabled(mgoRepo, n)
	if err != nil {
		return nil, fmt.Errorf("Unable to get checks, err: %v", err)
	}

	var serviceNamesToReturn []string
	for _, svc := range servicesToReturn {
		serviceNamesToReturn = append(serviceNamesToReturn, svc.Name)
	}

	collection := mgoRepo.Db().C(constants.HealthchecksCollection)

	pipeline := []bson.M{
		{"$match": bson.M{"service.name": bson.M{"$in": serviceNamesToReturn}, "service.namespace": n, "service.deployment.desiredReplicas": bson.M{"$gt": 0}}},
		{"$sort": bson.M{"checkTime": -1}},
		{"$group": bson.M{"_id": "$service.name", "checks": bson.M{"$first": "$$ROOT"}}},
		{"$replaceRoot": bson.M{"newRoot": "$checks"}}}

	pipe := collection.Pipe(pipeline).AllowDiskUse()

	var checks []model.ServiceStatus
	if err := pipe.All(&checks); err != nil {
		return nil, fmt.Errorf("failed to get all healthcheck responses for service within namespace %v err: %v", n, err)
	}

	if checks == nil {
		checks = []model.ServiceStatus{}
	}
	return checks, nil
}

// DeleteHealthchecksOlderThan deletes health check responses older than the given number of days
func DeleteHealthchecksOlderThan(removeAfterDays int, mgoRepo *MongoRepository) error {

	collection := mgoRepo.Db().C(constants.HealthchecksCollection)

	if _, err := collection.RemoveAll(bson.M{"checkTime": bson.M{"$lt": time.Now().AddDate(0, 0, -removeAfterDays)}}); err != nil {
		return err
	}

	return nil
}

// DropDB drops the database
func DropDB(mgoRepo *MongoRepository) error {
	return mgoRepo.Db().DropDatabase()
}

// GetHealthchecks retrieves the list of Services (and their health annotations) from the DB and places them on a channel
// of type Service
func GetHealthchecks(mgoRepo *MongoRepository, healthchecks chan model.Service, errs chan error, restrictToNamespace ...string) {
	services, err := FindAllServicesWithHealthScrapeEnabled(mgoRepo, restrictToNamespace...)
	if err != nil {
		select {
		case errs <- fmt.Errorf("Could not get services (%v)", err):
		default:
		}
		return
	}
	log.Debugf("Adding %v service to channel with %v elements\n", len(services), len(healthchecks))
	for _, s := range services {
		healthchecks <- s
	}
}

// RemoveOlderThan deletes health checks older than the given numnber of days
func RemoveOlderThan(removeAfterDays int, mgoRepo *MongoRepository, errs chan error) {
	err := DeleteHealthchecksOlderThan(removeAfterDays, mgoRepo)
	if err != nil {
		select {
		case errs <- fmt.Errorf("Could not delete old healthchecks (%v)", err):
		default:
		}
		return
	}
}

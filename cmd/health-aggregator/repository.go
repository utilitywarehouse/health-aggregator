package main

import (
	"fmt"
	"sync"

	"github.com/pkg/errors"

	"github.com/globalsign/mgo/bson"
	log "github.com/sirupsen/logrus"
)

func upsertServiceConfigs(mgoRepo *MongoRepository, services chan service, errs chan error) {

	var waitGroup sync.WaitGroup
	readers := 5

	// Perform 5 concurrent queries against the database.
	waitGroup.Add(readers)
	for i := 0; i < readers; i++ {
		go func(services chan service) {
			defer waitGroup.Done()
			for s := range services {
				repoCopy := mgoRepo.WithNewSession()
				defer repoCopy.Close()

				collection := repoCopy.db().C(servicesCollection)

				_, err := collection.Upsert(bson.M{"name": s.Name, "namespace": s.Namespace}, s)
				if err != nil {
					log.WithError(err).Errorf("failed to insert service %s in namespace %s", s.Name, s.Namespace)
					return
				}
			}
		}(services)
	}
	waitGroup.Wait()
}

func upsertNamespaceConfigs(mgoRepo *MongoRepository, namespaces chan namespace, errs chan error) {

	var waitGroup sync.WaitGroup
	writers := 5

	// Perform 5 concurrent queries against the database.
	waitGroup.Add(writers)
	for i := 0; i < writers; i++ {
		go func(namespaces chan namespace) {
			defer waitGroup.Done()
			for n := range namespaces {
				repoCopy := mgoRepo.WithNewSession()
				defer repoCopy.Close()

				collection := repoCopy.db().C(namespacesCollection)

				_, err := collection.Upsert(bson.M{"name": n.Name}, n)
				if err != nil {
					log.WithError(err).Errorf("failed to insert namespace %s", n.Name)
					return
				}
			}
		}(namespaces)
	}
	waitGroup.Wait()
}

func insertHealthcheckResponses(mgoRepo *MongoRepository, responses chan healthcheckResp, errs chan error) {

	writers := 5
	for i := 0; i < writers; i++ {
		go func(responses chan healthcheckResp) {
			for r := range responses {
				repoCopy := mgoRepo.WithNewSession()
				defer repoCopy.Close()

				collection := repoCopy.db().C(healthchecksCollection)

				err := collection.Insert(r)
				if err != nil {
					log.WithError(err).Errorf("failed to insert healthcheck response for %s for namespace %s", r.Service.Name, r.Service.Namespace)
					return
				}
			}
		}(responses)
	}
}

func findAllServices(mgoRepo *MongoRepository) ([]service, error) {
	repoCopy := mgoRepo.WithNewSession()
	defer repoCopy.Close()

	collection := repoCopy.db().C(servicesCollection)

	var services []service
	if err := collection.Find(bson.M{}).All(&services); err != nil {
		return nil, errors.New("failed to get all services")
	}

	return services, nil
}

func findAllNamespaces(mgoRepo *MongoRepository) ([]namespace, error) {
	repoCopy := mgoRepo.WithNewSession()
	defer repoCopy.Close()

	collection := repoCopy.db().C(namespacesCollection)

	var ns []namespace
	if err := collection.Find(bson.M{}).All(&ns); err != nil {
		return nil, errors.New("failed to get all namespaces")
	}

	if ns == nil {
		ns = []namespace{}
	}

	return ns, nil
}

func findAllServicesForNameSpace(mgoRepo *MongoRepository, ns string) ([]service, error) {
	repoCopy := mgoRepo.WithNewSession()
	defer repoCopy.Close()

	collection := repoCopy.db().C(servicesCollection)

	var svcs []service
	if err := collection.Find(bson.M{"namespace": ns}).All(&svcs); err != nil {
		return nil, fmt.Errorf("failed to get all services for namespace %s", ns)
	}

	if svcs == nil {
		svcs = []service{}
	}

	return svcs, nil
}

func findAllServicesWithHealthScrapeEnabled(mgoRepo *MongoRepository) ([]service, error) {
	repoCopy := mgoRepo.WithNewSession()
	defer repoCopy.Close()

	collection := repoCopy.db().C(servicesCollection)

	var svcs []service
	if err := collection.Find(bson.M{"healthAnnotations.enableScrape": "true"}).Sort("namespace").All(&svcs); err != nil {
		return nil, fmt.Errorf("failed to get all service healthcheck endpoints with scrape enabled")
	}

	return svcs, nil
}

func findAllChecksForService(mgoRepo *MongoRepository, n string, s string) ([]healthcheckResp, error) {
	repoCopy := mgoRepo.WithNewSession()
	defer repoCopy.Close()

	collection := repoCopy.db().C(healthchecksCollection)

	var checks []healthcheckResp
	if err := collection.Find(bson.M{"service.namespace": n, "service.name": s}).Limit(50).Sort("-checkTime").All(&checks); err != nil {
		return nil, fmt.Errorf("failed to get all healthcheck responses for service %v in namespace %v", s, n)
	}

	if checks == nil {
		checks = []healthcheckResp{}
	}

	return checks, nil
}

func findLatestChecksForNamespace(mgoRepo *MongoRepository, n string) ([]healthcheckResp, error) {
	repoCopy := mgoRepo.WithNewSession()
	defer repoCopy.Close()

	collection := repoCopy.db().C(healthchecksCollection)
	pipeline := []bson.M{
		{"$match": bson.M{"service.namespace": n}},
		{"$sort": bson.M{"checkTime": -1}},
		{"$group": bson.M{"_id": "$service.name", "checks": bson.M{"$push": "$$ROOT"}}},
		{"$replaceRoot": bson.M{"newRoot": bson.M{"$arrayElemAt": []interface{}{"$checks", 0}}}}}

	pipe := collection.Pipe(pipeline)

	var checks []healthcheckResp
	err := pipe.All(&checks)
	if err != nil {
		return nil, fmt.Errorf("failed to get all healthcheck responses for service within namespace %v", n)
	}

	if checks == nil {
		checks = []healthcheckResp{}
	}

	return checks, nil
}

package dbutils

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/utilitywarehouse/health-aggregator/internal/constants"
	"github.com/utilitywarehouse/health-aggregator/internal/db"
	"github.com/utilitywarehouse/health-aggregator/internal/model"
)

// InsertItems inserts any number of Services, Namespaces or HealthcheckResp into the DB
func InsertItems(mgoRepo *db.MongoRepository, objs ...interface{}) {
	for _, obj := range objs {
		InsertItem(mgoRepo, obj)
	}
}

// InsertItem inserts a single Service, Namespace or ServiceStatus into the DB
func InsertItem(mgoRepo *db.MongoRepository, obj interface{}) {
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

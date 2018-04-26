package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/mux"

	"github.com/globalsign/mgo"
	"github.com/jawher/mow.cli"
	log "github.com/sirupsen/logrus"
	"github.com/utilitywarehouse/go-operational-health-checks/healthcheck"
	"github.com/utilitywarehouse/go-operational/op"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

var (
	client = &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 128,
			Dial: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).Dial,
		},
	}
	outOfCluster bool
)

const (
	appName                = "health-aggregator"
	appDesc                = "This app aggregates the health of apps across k8s namespaces for a cluster."
	defaultEnableScrape    = "true"
	defaultPort            = "8080"
	servicesCollection     = "services"
	namespacesCollection   = "namespaces"
	healthchecksCollection = "checks"
	dbName                 = "healthaggregator"
)

var (
	gitHash string // populated at compile time
)

type healthcheckResp struct {
	Service    service         `json:"service" bson:"service"`
	CheckTime  time.Time       `json:"checkTime" bson:"checkTime"`
	State      string          `json:"state" bson:"state"`
	StateSince time.Time       `json:"stateSince" bson:"stateSince"`
	StatusCode int             `json:"statusCode" bson:"statusCode"`
	Error      string          `json:"error" bson:"error"`
	Body       healthcheckBody `json:"healthcheckBody,omitempty" bson:"healthcheckBody"`
}

type healthcheckBody struct {
	Name        string  `json:"name" bson:"name"`
	Description string  `json:"description" bson:"description"`
	Health      string  `json:"health" bson:"health"`
	Checks      []check `json:"checks" bson:"checks"`
}

type check struct {
	Name   string `json:"name" bson:"name"`
	Health string `json:"health" bson:"health"`
	Output string `json:"output,omitempty" bson:"output"`
	Action string `json:"action,omitempty" bson:"action"`
	Impact string `json:"impact,omitempty" bson:"impact"`
}

type reloadHandler struct {
	discovery *serviceDiscovery
}

func main() {
	app := cli.App("health-aggegrator", "Calls /__/health for services that expose the endpoint and aggregates the responses")
	port := app.String(cli.StringOpt{
		Name:   "port",
		Desc:   "Port to listen on",
		EnvVar: "PORT",
		Value:  "8080",
	})
	opsPort := app.Int(cli.IntOpt{
		Name:   "ops-port",
		Desc:   "The HTTP ops port",
		EnvVar: "OPS_PORT",
		Value:  8081,
	})
	kubeConfigPath := app.String(cli.StringOpt{
		Name:   "kubeconfig",
		Desc:   "(optional) absolute path to the kubeconfig file",
		EnvVar: "KUBECONFIG_FILEPATH",
		Value:  "",
	})
	logLevel := app.String(cli.StringOpt{
		Name:   "log-level",
		Desc:   "Log level (e.g. INFO, DEBUG, WARN)",
		EnvVar: "LOG_LEVEL",
		Value:  "INFO",
	})
	dbURL := app.String(cli.StringOpt{
		Name:   "mongo-connection-string",
		Desc:   "Connection string to connect to mongo ex mongodb:27017/",
		EnvVar: "MONGO_CONNECTION_STRING",
		Value:  "127.0.0.1:27017/",
	})
	dropDB := app.Bool(cli.BoolOpt{
		Name:   "mongo-drop-db",
		Desc:   "Set to true in order to drop the DB on startup",
		EnvVar: "MONGO_DROP_DB",
		Value:  false,
	})
	removeAfterDays := app.Int(cli.IntOpt{
		Name:   "delete-checks-after-days",
		Desc:   "Age of check results in days after which they are deleted",
		EnvVar: "DELETE_CHECKS_AFTER_DAYS",
		Value:  7,
	})

	app.Before = func() {
		setLogger(logLevel)
	}

	app.Action = func() {

		mgoSess, err := mgo.DialWithTimeout(*dbURL, 1*time.Second)
		if err != nil {
			log.WithError(err).Panic("failed to connect to mongo")
		}
		mgoRepo := NewMongoRepository(mgoSess, dbName)
		defer mgoSess.Close()

		dropDatabase(*dropDB, mgoRepo)

		kubeClient := newKubeClient(*kubeConfigPath)

		errs := make(chan error, 10)
		namespaces := make(chan namespace, 10)
		services := make(chan service, 10)
		healthchecks := make(chan service, 1000)
		responses := make(chan healthcheckResp, 1000)

		s := &serviceDiscovery{client: kubeClient.client, label: "app", namespaces: namespaces, services: services, errors: errs}
		reloader := reloadHandler{discovery: s}

		go initHTTPServer(*opsPort, mgoSess)
		go s.getClusterHealthcheckConfig()
		go upsertNamespaceConfigs(mgoRepo, namespaces, errs)
		go upsertServiceConfigs(mgoRepo, services, errs)

		c := newHealthChecker()

		ticker := time.NewTicker(60 * time.Second)
		go func() {
			for t := range ticker.C {
				log.Printf("Scheduling healthchecks at %v", t)
				getHealthchecks(mgoRepo, healthchecks, errs)
			}
		}()

		tidyTicker := time.NewTicker(60 * time.Minute)
		go func() {
			for t := range tidyTicker.C {
				log.Printf("Tidying old healthchecks %v", t)
				removeHealthchecksOlderThan(*removeAfterDays, mgoRepo, errs)
			}
		}()

		go c.doHealthchecks(healthchecks, responses, errs)
		go insertHealthcheckResponses(mgoRepo, responses, errs)

		go func() {
			for e := range errs {
				log.Printf("ERROR: %v", e)
			}
		}()

		r := mux.NewRouter()
		r.HandleFunc("/reload", reloader.reload()).Methods("POST")
		r.HandleFunc("/services", getAllServices(mgoRepo)).Methods(http.MethodGet)
		r.HandleFunc("/namespaces", getAllNamespaces(mgoRepo)).Methods(http.MethodGet)
		r.HandleFunc("/namespaces/{namespace}/services", getServicesForNameSpace(mgoRepo)).Methods(http.MethodGet)
		r.HandleFunc("/namespaces/{namespace}/services/{service}/checks", getAllChecksForService(mgoRepo)).Methods(http.MethodGet)
		r.HandleFunc("/namespaces/{namespace}/services/checks", getLatestChecksForNamespace(mgoRepo)).Methods(http.MethodGet)

		log.Printf("Listening on [%v].\n", *port)
		err = http.ListenAndServe(":"+*port, r)
		if err != nil {
			log.Fatalf("ERROR: Web server failed: error=(%v).\n", err)
		}
	}
	app.Run(os.Args)
}

func (h reloadHandler) reload() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		go h.discovery.getClusterHealthcheckConfig()
		responseWithJSON(w, []byte("{\"message\": \"ok\"}"), http.StatusOK)
	}
}

func setLogger(logLevel *string) {
	log.SetFormatter(&log.JSONFormatter{})
	lvl, err := log.ParseLevel(*logLevel)
	if err != nil {
		log.WithError(err).Fatal("Error parsing log level")
	}
	log.SetLevel(lvl)
}

func dropDatabase(dropDB bool, mgoRepo *MongoRepository) {
	err := mgoRepo.session.DB(dbName).DropDatabase()
	if err != nil {
		log.WithError(err).Panic("failed to drop database")
		return
	}
	log.Info("drop database successful")
}

func initHTTPServer(opsPort int, mgoSess *mgo.Session) {
	if err := http.ListenAndServe(fmt.Sprintf(":%d", opsPort), op.NewHandler(op.
		NewStatus(appName, appDesc).
		AddOwner("labs", "#labs").
		AddLink("vcs", fmt.Sprintf("github.com/utilitywarehouse/health-aggegrator")).
		SetRevision(gitHash).
		AddChecker("mongo", healthcheck.NewMongoHealthCheck(mgoSess, "Unable to access mongo db")).
		ReadyUseHealthCheck().
		WithInstrumentedChecks(),
	)); err != nil {
		log.WithError(err).Fatal("ops server has shut down")
	}
}

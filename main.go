package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	h "github.com/gorilla/handlers"

	"github.com/globalsign/mgo"
	"github.com/google/uuid"
	"github.com/jawher/mow.cli"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"github.com/utilitywarehouse/go-operational-health-checks/healthcheck"
	"github.com/utilitywarehouse/go-operational/op"
	"github.com/utilitywarehouse/health-aggregator/internal/checks"
	"github.com/utilitywarehouse/health-aggregator/internal/constants"
	"github.com/utilitywarehouse/health-aggregator/internal/db"
	"github.com/utilitywarehouse/health-aggregator/internal/discovery"
	"github.com/utilitywarehouse/health-aggregator/internal/handlers"
	"github.com/utilitywarehouse/health-aggregator/internal/httpserver"
	"github.com/utilitywarehouse/health-aggregator/internal/instrumentation"
	"github.com/utilitywarehouse/health-aggregator/internal/model"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

var (
	gitHash string // populated at compile time
)

func main() {
	app := cli.App("health-aggegrator", "Calls /__/health for services that expose the endpoint and aggregates the responses")
	port := app.Int(cli.IntOpt{
		Name:   "port",
		Desc:   "Port to listen on",
		EnvVar: "PORT",
		Value:  8080,
	})
	opsPort := app.Int(cli.IntOpt{
		Name:   "ops-port",
		Desc:   "The HTTP ops port",
		EnvVar: "OPS_PORT",
		Value:  8081,
	})
	writeTimeout := app.Int(cli.IntOpt{
		Name:   "write-timeout",
		Desc:   "The WriteTimeout for HTTP connections",
		EnvVar: "HTTP_WRITE_TIMEOUT",
		Value:  15,
	})
	readTimeout := app.Int(cli.IntOpt{
		Name:   "read-timeout",
		Desc:   "The ReadTimeout for HTTP connections",
		EnvVar: "HTTP_READ_TIMEOUT",
		Value:  15,
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
		Value:  1,
	})
	restrictToNamespaces := app.Strings(cli.StringsOpt{
		Name:   "restrict-namespace",
		Desc:   "Restrict checks to one or more namespaces - e.g. export RESTRICT_NAMESPACE=\"auth\",\"redis\"",
		EnvVar: "RESTRICT_NAMESPACE",
		Value:  []string{},
	})
	kubeConfigPath := app.String(cli.StringOpt{
		Name:   "kubeconfig",
		Desc:   "(optional) absolute path to the kubeconfig file",
		EnvVar: "KUBECONFIG_FILEPATH",
		Value:  "",
	})

	app.Before = func() {
		setLogger(logLevel)
	}

	app.Action = func() {
		log.Debug("dialling mongo")

		// Create new session
		mgoSess := newMongoSession(*dbURL)
		defer mgoSess.Close()

		// Create new repository
		mgoRepo := createMongoRepoAndIndex(mgoSess, *dropDB, constants.DBName)

		// Set up services state
		servicesState, stateErr := db.GetServicesState(mgoRepo)
		if stateErr != nil {
			log.Panicf("unable to load services state: %v", stateErr)
		}

		errs := make(chan error, 10)
		updateItems := make(chan model.UpdateItem, 10)

		// Create new kube client
		kubeClient := discovery.NewKubeClient(*kubeConfigPath)

		// Create new discoveryService - responsible for watching k8s deployments and getting
		// Namespace and Service annotations
		discoveryService := discovery.NewKubeDiscoveryService(kubeClient, servicesState, updateItems, errs)

		// Watch for updates to deployments for known k8s namespaces and add
		// updated objects to the updateItems channel
		go discoveryService.WatchDeployments(*restrictToNamespaces)

		// Create new updaterService - listens for objects to update - updateItems are put on the
		// channel by the k8s deployments watcher (discoveryService.WatchDeployments)
		updaterService := db.NewUpdaterService(updateItems, errs, mgoRepo)

		// Persist any objects added to the updateItems channel
		go updaterService.DoUpdates()

		// The reloadQueue receives a request UUID.
		// Items on the reloadQueue triggers the retrieval of the latest Namespace and Service
		// health-aggregator annotations and persists them in the data store.
		reloadQueue := make(chan uuid.UUID)

		// Range over the reload queue (persists k8s services and namespaces configs)
		go discoveryService.ReloadServiceConfigs(reloadQueue, mgoRepo)

		// Place a new request (UUID) onto the reload queue every 60 minutes.
		reloadTicker := time.NewTicker(constants.ReloadServicesIntervalMins * time.Minute)
		go func() {
			for t := range reloadTicker.C {

				log.Infof("scheduling reload of k8s annotations at %v", t)
				reloadQueue <- uuid.New()
			}
		}()

		// Schedule deletion services that were not updated in recent reloads
		serviceTidyTicker := time.NewTicker((constants.ReloadServicesIntervalMins) * time.Minute)
		go func() {
			for t := range serviceTidyTicker.C {
				log.Infof("tidying stale services %v", t)
				db.RemoveStaleServices(mgoRepo, errs)
			}
		}()

		metrics := instrumentation.SetupMetrics()

		// Schedule health check scraping every 60 seconds
		servicesToScrape := make(chan model.Service, 1000)
		ticker := time.NewTicker(60 * time.Second)
		go func() {
			for t := range ticker.C {
				log.Infof("scheduling healthchecks at %v", t)
				db.GetHealthchecks(mgoRepo, servicesToScrape, errs, metrics, *restrictToNamespaces...)
			}
		}()

		// Schedule deletion of older health checks every...
		tidyTicker := time.NewTicker(60 * time.Minute)
		go func() {
			for t := range tidyTicker.C {
				log.Infof("tidying old healthchecks %v", t)
				db.RemoveChecksOlderThan(*removeAfterDays, mgoRepo, errs)
			}
		}()

		// Channel used to store the status of a health check response
		statusResponses := make(chan model.ServiceStatus, 1000)

		// Scrape health check endpoints for services that appear on the servicesToScrape channel
		// and send responses to the statusResponses chan
		healthChecker := checks.NewHealthChecker(kubeClient, metrics, "")
		go healthChecker.DoHealthchecks(servicesToScrape, statusResponses, errs)

		// Insert health check reponses into mongo that appear on the statusResponses chan
		go db.InsertHealthcheckResponses(mgoRepo, statusResponses, errs, metrics)

		// Log any errors that appear on the errs chan
		go func() {
			for e := range errs {
				log.Errorf("%v", e)
			}
		}()

		// Set up routes and start API
		router := handlers.NewRouter(reloadQueue)
		allowedCORSMethods := h.AllowedMethods([]string{http.MethodPost, http.MethodOptions})
		allowedCORSOrigins := h.AllowedOrigins([]string{"*"})
		server := httpserver.New(*port, router, *writeTimeout, *readTimeout, allowedCORSMethods, allowedCORSOrigins)
		go httpserver.Start(server)

		// Start the Ops HTTP server
		go initOpsHTTPServer(*opsPort, mgoSess, metrics)

		graceful(server, 10)
	}
	app.Run(os.Args)
}

func newMongoSession(dbURL string) *mgo.Session {
	mgoSess, err := mgo.Dial(dbURL)
	if err != nil {
		log.WithError(err).Panicf("failed to connect to mongo using connection string %v", dbURL)
	}
	return mgoSess
}

func createMongoRepoAndIndex(mgoSess *mgo.Session, dropDB bool, dbName string) *db.MongoRepository {

	mgoRepo := db.NewMongoRepository(mgoSess, dbName)

	if dropDB {
		log.Info("dropping database")
		dropErr := db.DropDB(mgoRepo)
		if dropErr != nil {
			log.WithError(dropErr).Panic("failed to drop database")
		}
		log.Info("drop database successful")
	}

	createIndex(mgoRepo)

	return mgoRepo
}

func graceful(hs *http.Server, timeout time.Duration) {
	stop := make(chan os.Signal, 1)

	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Info("Shutting down ")

	if err := hs.Shutdown(ctx); err != nil {
		log.WithError(err).Error("Error shutting down server")
	} else {
		log.Info("Server stopped")
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

func createIndex(mgoRepo *db.MongoRepository) {
	log.Debugf("creating mongodb index for collection %v", constants.HealthchecksCollection)
	c := mgoRepo.Db().C(constants.HealthchecksCollection)

	index := mgo.Index{
		Key: []string{"-checkTime"},
	}

	err := c.EnsureIndex(index)
	if err != nil {
		panic(err)
	}
	log.Debug("index creation successful")
}

func initOpsHTTPServer(opsPort int, mgoSess *mgo.Session, metrics instrumentation.Metrics) {
	log.Info("starting ops server")

	promMetrics := []prometheus.Collector{}
	for _, cv := range metrics.Counters {
		promMetrics = append(promMetrics, cv)
	}
	for _, gv := range metrics.Gauges {
		promMetrics = append(promMetrics, gv)
	}
	for _, hv := range metrics.Histograms {
		promMetrics = append(promMetrics, hv)
	}
	if err := http.ListenAndServe(fmt.Sprintf(":%d", opsPort), op.NewHandler(op.
		NewStatus(constants.AppName, constants.AppDesc).
		AddOwner("labs", "#labs").
		AddLink("vcs", fmt.Sprintf("github.com/utilitywarehouse/health-aggegrator")).
		SetRevision(gitHash).
		AddChecker("mongo", healthcheck.NewMongoHealthCheck(mgoSess, "Unable to access mongo db")).
		AddMetrics(promMetrics...).
		ReadyAlways().
		WithInstrumentedChecks(),
	)); err != nil {
		log.WithError(err).Fatal("ops server has shut down")
	}
}

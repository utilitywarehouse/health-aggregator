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
	"github.com/utilitywarehouse/health-aggregator/internal/model"
	"github.com/utilitywarehouse/health-aggregator/internal/statuspage"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

var (
	gitHash           string // populated at compile time
	statusPageBaseURL = "https://api.statuspage.io/v1"
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
	statusPageIOPageID := app.String(cli.StringOpt{
		Name:   "statuspage-io-page-id",
		Desc:   "The Page ID for the statuspage.io page for which health aggregator will update the state of components",
		EnvVar: "STATUSPAGE_IO_PAGE_ID",
		Value:  "",
	})
	statusPageIOAPIKey := app.String(cli.StringOpt{
		Name:   "statuspage-io-api-key",
		Desc:   "The API key for statuspage.io",
		EnvVar: "STATUSPAGE_IO_API_KEY",
		Value:  "",
	})
	updateStatuspageIO := app.Bool(cli.BoolOpt{
		Name:   "update-statuspage-io",
		Desc:   "Set to true in order to perform updates to statuspage.io components",
		EnvVar: "UPDATE_STATUSPAGE_IO",
		Value:  false,
	})

	app.Before = func() {
		setLogger(logLevel)
	}

	app.Action = func() {
		log.Debug("dialling mongo")

		mgoSess, err := mgo.Dial(*dbURL)
		if err != nil {
			log.WithError(err).Panicf("failed to connect to mongo using connection string %v", *dbURL)
		}
		defer mgoSess.Close()

		mgoRepo := db.NewMongoRepository(mgoSess, constants.DBName)

		// Drop the database if required
		if *dropDB {
			log.Info("dropping database")
			dropErr := db.DropDB(mgoRepo)
			if dropErr != nil {
				log.WithError(err).Panic("failed to drop database")
				return
			}
			log.Info("drop database successful")
		}

		createIndex(mgoRepo)

		// Make all required channels
		errs := make(chan error, 10)
		servicesToScrape := make(chan model.Service, 1000)
		statusResponses := make(chan model.ServiceStatus, 1000)

		kubeClient := discovery.NewKubeClient(*kubeConfigPath)

		router := handlers.NewRouter(mgoRepo, kubeClient)
		allowedCORSMethods := h.AllowedMethods([]string{http.MethodGet, http.MethodPost, http.MethodOptions})
		allowedCORSOrigins := h.AllowedOrigins([]string{"*"})
		server := httpserver.New(*port, router, *writeTimeout, *readTimeout, allowedCORSMethods, allowedCORSOrigins)
		go httpserver.Start(server)

		// Start the ops HTTP server
		metrics := setupMetrics()
		go initOpsHTTPServer(*opsPort, mgoSess, metrics)

		// Schedule service health check scraping every X seconds and add services to scrape to the
		// servicesToScrape channel
		ticker := time.NewTicker(60 * time.Second)
		go func() {
			for t := range ticker.C {
				log.Infof("Scheduling healthchecks at %v", t)
				db.GetHealthchecks(mgoRepo, servicesToScrape, errs, *restrictToNamespaces...)
			}
		}()

		// Schedule deletion of older health checks every...
		tidyTicker := time.NewTicker(60 * time.Minute)
		go func() {
			for t := range tidyTicker.C {
				log.Infof("tidying old healthchecks %v", t)
				db.RemoveOlderThan(*removeAfterDays, mgoRepo, errs)
			}
		}()

		// Scrape health checks that appear on the servicesToScrape chan, send responses to the statusResponses
		// chan
		healthChecker := checks.NewHealthChecker(kubeClient, metrics, "")
		go healthChecker.DoHealthchecks(servicesToScrape, statusResponses, errs)

		statusPageUpdater := statuspage.NewStatusPageUpdater(statusPageBaseURL, *statusPageIOPageID, *statusPageIOAPIKey, *updateStatuspageIO)
		// Insert health check reponses into mongo that appear on the statusResponses chan, and send
		// components that require updating on statuspage.io to the statuspageIOComponents chan
		go db.InsertHealthcheckResponses(mgoRepo, statusResponses, statusPageUpdater, errs)

		// Log out any errors that appear on the errs chan
		go func() {
			for e := range errs {
				log.Printf("ERROR: %v", e)
			}
		}()

		graceful(server, 10)
	}
	app.Run(os.Args)
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

func setupMetrics() checks.Metrics {
	var metrics checks.Metrics

	metrics.Counters = setupCounters()
	metrics.Gauges = setupGauges()

	return metrics
}

func setupCounters() map[string]*prometheus.CounterVec {

	counters := make(map[string]*prometheus.CounterVec)

	counters[constants.HealthAggregatorOutcome] = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: constants.HealthAggregatorOutcome,
		Help: "Counts health checks performed including the outcome (whether or not the healthcheck call was successful or not)",
	}, []string{constants.PerformedHealthcheckResult})

	return counters
}

func setupGauges() map[string]*prometheus.GaugeVec {

	gauges := make(map[string]*prometheus.GaugeVec)

	gauges[constants.HealthAggregatorInFlight] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: constants.HealthAggregatorInFlight,
		Help: "Records the number of health checks which are in flight at any one time",
	}, []string{})

	return gauges
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

func initOpsHTTPServer(opsPort int, mgoSess *mgo.Session, metrics checks.Metrics) {
	log.Debug("starting ops server")

	promMetrics := []prometheus.Collector{}
	for _, cv := range metrics.Counters {
		promMetrics = append(promMetrics, cv)
	}
	for _, gv := range metrics.Gauges {
		promMetrics = append(promMetrics, gv)
	}
	if err := http.ListenAndServe(fmt.Sprintf(":%d", opsPort), op.NewHandler(op.
		NewStatus(constants.AppName, constants.AppDesc).
		AddOwner("labs", "#labs").
		AddLink("vcs", fmt.Sprintf("github.com/utilitywarehouse/health-aggegrator")).
		SetRevision(gitHash).
		AddChecker("mongo", healthcheck.NewMongoHealthCheck(mgoSess, "Unable to access mongo db")).
		AddMetrics(promMetrics...).
		ReadyUseHealthCheck().
		WithInstrumentedChecks(),
	)); err != nil {
		log.WithError(err).Fatal("ops server has shut down")
	}
}

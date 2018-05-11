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

	app.Before = func() {
		setLogger(logLevel)
	}

	app.Action = func() {
		fmt.Println(*restrictToNamespaces)
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

		kubeClient := discovery.NewKubeClient()

		errs := make(chan error, 10)
		namespaces := make(chan model.Namespace, 10)
		services := make(chan model.Service, 10)
		healthchecks := make(chan model.Service, 1000)
		responses := make(chan model.HealthcheckResp, 1000)

		s := &discovery.ServiceDiscovery{Client: kubeClient.Client, Label: "app", Namespaces: namespaces, Services: services, Errors: errs}

		router := handlers.NewRouter(s, mgoRepo)
		allowedCORSMethods := h.AllowedMethods([]string{http.MethodGet, http.MethodPost, http.MethodOptions})
		allowedCORSOrigins := h.AllowedOrigins([]string{"*"})
		server := httpserver.New(*port, router, *writeTimeout, *readTimeout, allowedCORSMethods, allowedCORSOrigins)
		go httpserver.Start(server)
		go initHTTPServer(*opsPort, mgoSess)

		go s.GetClusterHealthcheckConfig()
		go db.UpsertNamespaceConfigs(mgoRepo.WithNewSession(), namespaces, errs)
		go db.UpsertServiceConfigs(mgoRepo.WithNewSession(), services, errs)

		c := checks.NewHealthChecker()

		ticker := time.NewTicker(60 * time.Second)
		go func() {
			for t := range ticker.C {
				log.Infof("Scheduling healthchecks at %v", t)
				db.GetHealthchecks(mgoRepo, healthchecks, errs, *restrictToNamespaces...)
			}
		}()

		tidyTicker := time.NewTicker(60 * time.Minute)
		go func() {
			for t := range tidyTicker.C {
				log.Infof("tidying old healthchecks %v", t)
				db.RemoveOlderThan(*removeAfterDays, mgoRepo, errs)
			}
		}()

		go c.DoHealthchecks(healthchecks, responses, errs)
		go db.InsertHealthcheckResponses(mgoRepo, responses, errs)

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

func initHTTPServer(opsPort int, mgoSess *mgo.Session) {
	log.Debug("starting ops server")
	if err := http.ListenAndServe(fmt.Sprintf(":%d", opsPort), op.NewHandler(op.
		NewStatus(constants.AppName, constants.AppDesc).
		AddOwner("labs", "#labs").
		AddLink("vcs", fmt.Sprintf("github.com/utilitywarehouse/health-aggegrator")).
		SetRevision(gitHash).
		AddChecker("mongo", healthcheck.NewMongoHealthCheck(mgoSess, "Unable to access mongo db")).
		ReadyAlways().
		WithInstrumentedChecks(),
	)); err != nil {
		log.WithError(err).Fatal("ops server has shut down")
	}
}

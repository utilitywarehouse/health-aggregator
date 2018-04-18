package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/globalsign/mgo"
	"github.com/gorilla/mux"
	"github.com/jawher/mow.cli"
	log "github.com/sirupsen/logrus"
	"github.com/utilitywarehouse/go-operational-health-checks/healthcheck"
	"github.com/utilitywarehouse/go-operational/op"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

var client = &http.Client{
	Transport: &http.Transport{
		MaxIdleConnsPerHost: 128,
		Dial: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
	},
}

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
	dropDB  = true
	gitHash string // populated at compile time
)

type healthcheckResp struct {
	Service    service         `json:"service" bson:"service"`
	CheckTime  time.Time       `json:"checkTime" bson:"checkTime"`
	StatusCode int             `json:"statusCode" bson:"statusCode"`
	Error      string          `json:"error" bson:"error"`
	Body       healthcheckBody `json:"healthcheckBody" bson:"healthcheckBody"`
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
	Output string `json:"output" bson:"output"`
	Action string `json:"action" bson:"action"`
	Impact string `json:"impact" bson:"impact"`
}

type handler struct {
	discovery *serviceDiscovery
}

func main() {
	app := cli.App("health-aggegrator", "Calls /__/health for services that expose the endpoint and aggregates the responses")
	port := app.String(cli.StringOpt{
		Name:   "port",
		Value:  "8080",
		Desc:   "Port to listen on",
		EnvVar: "PORT",
	})
	opsPort := app.Int(cli.IntOpt{
		Name:   "ops-port",
		Desc:   "The HTTP ops port",
		EnvVar: "OPS_PORT",
		Value:  8081,
	})
	kubeconfig := app.String(cli.StringOpt{
		Name:   "kubeconfig",
		Value:  "",
		Desc:   "(optional) absolute path to the kubeconfig file",
		EnvVar: "KUBECONFIG_FILEPATH",
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
	kubernetesHost := app.String(cli.StringOpt{
		Name:   "kubernetes-service-host",
		Value:  "",
		Desc:   "Kubernetes service host",
		EnvVar: "KUBERNETES_SERVICE_HOST",
	})
	kubernetesPort := app.String(cli.StringOpt{
		Name:   "kubernetes-service-port",
		Value:  "",
		Desc:   "Kubernetes service port",
		EnvVar: "KUBERNETES_SERVICE_PORT",
	})
	kubernetesTokenPath := app.String(cli.StringOpt{
		Name:   "kubernetes-token-path",
		Value:  "/var/run/secrets/kubernetes.io/serviceaccount/token",
		Desc:   "Path to the kubernetes api token",
		EnvVar: "KUBERNETES_TOKEN_PATH",
	})
	kubernetesCertPath := app.String(cli.StringOpt{
		Name:   "kubernetes-cert-path",
		Value:  "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
		Desc:   "Path to the kubernetes cert",
		EnvVar: "KUBERNETES_CERT_PATH",
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

		if dropDB {
			err = mgoRepo.session.DB(dbName).DropDatabase()
			if err != nil {
				log.WithError(err).Panic("failed to drop database")
			}
		}

		kubeClient := newKubeClient(*kubeconfig, *kubernetesHost, *kubernetesPort, *kubernetesTokenPath, *kubernetesCertPath)

		errs := make(chan error, 10)
		namespaces := make(chan namespace, 10)
		services := make(chan service, 10)
		healthchecks := make(chan service, 1000)
		responses := make(chan healthcheckResp, 1000)

		s := &serviceDiscovery{client: kubeClient.client, label: "app", namespaces: namespaces, services: services, errors: errs}
		h := handler{discovery: s}

		go initHTTPServer(*opsPort, mgoSess)
		go s.getClusterHealthcheckConfig()
		go upsertNamespaceConfigs(mgoRepo, namespaces, errs)
		go upsertServiceConfigs(mgoRepo, services, errs)

		ticker := time.NewTicker(60 * time.Second)
		c := newHealthChecker()
		go func() {
			for t := range ticker.C {
				log.Printf("Scheduling healthchecks at %v", t)
				getHealthchecks(mgoRepo, healthchecks, errs)
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
		r.HandleFunc("/services", getAllServices(mgoRepo)).Methods("GET")
		r.HandleFunc("/namespaces", getAllNamespaces(mgoRepo)).Methods("GET")
		r.HandleFunc("/namespaces/{namespace}/services", getServicesForNameSpace(mgoRepo)).Methods("GET")
		r.HandleFunc("/reload", h.reload()).Methods("POST")
		r.HandleFunc("/services/{service}/checks", getChecksForService(mgoRepo)).Methods("GET")

		log.Printf("Listening on [%v].\n", *port)
		err = http.ListenAndServe(":"+*port, r)
		if err != nil {
			log.Fatalf("ERROR: Web server failed: error=(%v).\n", err)
		}
	}
	app.Run(os.Args)
}

func (h handler) reload() func(w http.ResponseWriter, r *http.Request) {
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

func initHTTPServer(opsPort int, mgoSess *mgo.Session) {
	if err := http.ListenAndServe(fmt.Sprintf(":%d", opsPort), op.NewHandler(op.
		NewStatus(appName, appDesc).
		AddOwner("labs", "#labs").
		AddLink("vcs", fmt.Sprintf("github.com/utilitywarehouse/health-aggegrator")).
		SetRevision(gitHash).
		AddChecker("mongo", healthcheck.NewMongoHealthCheck(mgoSess, "Unable to access mongo db")).
		ReadyUseHealthCheck(),
	)); err != nil {
		log.WithError(err).Fatal("ops server has shut down")
	}
}

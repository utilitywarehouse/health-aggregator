package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"k8s.io/client-go/kubernetes"

	"github.com/alecthomas/template"
	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/utilitywarehouse/health-aggregator/internal/db"
	"github.com/utilitywarehouse/health-aggregator/internal/discovery"
	"github.com/utilitywarehouse/health-aggregator/internal/model"
)

type reloadHandler struct {
	discovery  *discovery.K8sDiscovery
	services   chan model.Service
	namespaces chan model.Namespace
}

type byStateThenByName []model.ServiceStatus

func (a byStateThenByName) Len() int      { return len(a) }
func (a byStateThenByName) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byStateThenByName) Less(i, j int) bool {
	if a[i].StatePriority < a[j].StatePriority {
		return true
	}
	if a[i].StatePriority > a[j].StatePriority {
		return false
	}
	return a[i].Service.Name < a[j].Service.Name
}

// NewRouter returned a *mux.Router and sets up all required routes and handlers
func NewRouter(mgoRepo *db.MongoRepository, kubeClient *kubernetes.Clientset) *mux.Router {
	r := mux.NewRouter()

	namespaces := make(chan model.Namespace, 10)
	services := make(chan model.Service, 10)
	errs := make(chan error, 10)

	s := &discovery.K8sDiscovery{K8sClient: kubeClient, Namespaces: namespaces, Services: services, Errors: errs}
	reloader := reloadHandler{discovery: s, namespaces: namespaces, services: services}

	r.Handle("/reload", withRepoCopy(mgoRepo, reloader.reload)).Methods(http.MethodPost)
	r.Handle("/services", withRepoCopy(mgoRepo, getAllServices)).Methods(http.MethodGet)
	r.Handle("/kube-ops/ready", yo()).Methods(http.MethodGet)
	r.Handle("/", renderChecks()).Methods(http.MethodGet)
	r.Handle("/namespaces", withRepoCopy(mgoRepo, getAllNamespaces)).Methods(http.MethodGet)
	r.Handle("/namespaces/{namespace}/services", withRepoCopy(mgoRepo, getServicesForNameSpace)).Methods(http.MethodGet)
	r.Handle("/namespaces/{namespace}/services/{service}/checks", withRepoCopy(mgoRepo, getAllChecksForService)).Methods(http.MethodGet)
	r.Handle("/namespaces/{namespace}/services/checks", withRepoCopy(mgoRepo, getLatestChecksForNamespace)).Methods(http.MethodGet)

	return r
}

func withRepoCopy(mgoRepo *db.MongoRepository, next func(mgoRepo *db.MongoRepository) http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repoCopy := mgoRepo.WithNewSession()
		defer repoCopy.Close()
		next(repoCopy).ServeHTTP(w, r)
	})
}

func (h reloadHandler) reload(mgoRepo *db.MongoRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		go func(errs chan error) {
			for e := range errs {
				log.Printf("ERROR: %v", e)
			}
		}(h.discovery.Errors)

		// Create updaters for namespaces and services
		servicesUpdater := db.NewK8sServicesConfigUpdater(h.discovery.Services, mgoRepo.WithNewSession())
		namespacesUpdater := db.NewK8sNamespacesConfigUpdater(h.discovery.Namespaces, mgoRepo.WithNewSession())

		go func() {
			namespacesUpdater.UpsertNamespaceConfigs()
		}()

		go func() {
			servicesUpdater.UpsertServiceConfigs()
		}()

		// Get the cluster configs from k8s and places them on the namespace and service channels
		go func() {
			h.discovery.GetClusterHealthcheckConfig()
			close(h.namespaces)
			close(h.services)
		}()

		responseWithJSON(w, http.StatusOK, map[string]string{"message": "ok"})
	}
}

func yo() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		fmt.Fprint(w, "Yo!")
		return
	})
}

func renderChecks() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var checkData model.TemplatedChecks

		ns, ok := r.URL.Query()["ns"]

		if !ok || len(ns) < 1 {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(200)
			fmt.Fprint(w, "Please enter a namespace in the 'ns' query string parameter")
			return
		} else {
			checkData.Namespace = ns[0]
		}

		zoom, ok := r.URL.Query()["zoom"]

		if !ok || len(zoom) < 1 {
			checkData.Zoom = "1.0"
		} else {
			checkData.Zoom = zoom[0]
		}

		bigScreen, ok := r.URL.Query()["bigscreen"]

		if !ok || len(bigScreen) < 1 {
			checkData.BigScreen = false
		} else {
			if bigScreen[0] == "true" {
				checkData.BigScreen = true
			}
		}

		tmpl, tmplErr := template.ParseFiles("internal/templates/nschecks.html")
		if tmplErr != nil {
			log.WithError(errors.Wrap(tmplErr, "failed to parse template")).Error()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(500)
			fmt.Fprint(w, "Failed to parse template")
			return
		}
		tmpl.Execute(w, checkData)
	})
}

func getAllNamespaces(mgoRepo *db.MongoRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ns, err := db.FindAllNamespaces(mgoRepo)
		if err != nil {
			log.WithError(err).Errorf("database error - failed to get all namespaces")
			errorWithJSON(w, "Database error", http.StatusInternalServerError)
			return
		}

		responseWithJSON(w, http.StatusOK, ns)
	}
}

func getAllServices(mgoRepo *db.MongoRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svcs, err := db.FindAllServices(mgoRepo)
		if err != nil {
			log.WithError(err).
				Errorf("database error - failed to get all services")
			errorWithJSON(w, "Database error", http.StatusInternalServerError)
			return
		}

		responseWithJSON(w, http.StatusOK, svcs)
	}
}

func getServicesForNameSpace(mgoRepo *db.MongoRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		ns := vars["namespace"]
		svcs, err := db.FindAllServicesForNamespace(mgoRepo, ns)
		if err != nil {
			log.WithField("namespace", ns).
				WithError(err).
				Errorf("database error - failed to get services for namespace")
			errorWithJSON(w, "Database error", http.StatusInternalServerError)
			return
		}

		responseWithJSON(w, http.StatusOK, svcs)
	}
}

func getAllChecksForService(mgoRepo *db.MongoRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		n := vars["namespace"]
		s := vars["service"]

		checks, err := db.FindAllChecksForService(mgoRepo, n, s)
		if err != nil {
			log.WithField("service", s).
				WithError(err).
				Errorf("Database error")
			errorWithJSON(w, "Database error", http.StatusInternalServerError)
			return
		}

		responseWithJSON(w, http.StatusOK, checks)
	}
}

func getLatestChecksForNamespace(mgoRepo *db.MongoRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		n := vars["namespace"]

		checks, err := db.FindLatestChecksForNamespace(mgoRepo, n)
		if err != nil {
			log.WithField("namespace", n).
				WithError(err).
				Errorf("Database error")
			errorWithJSON(w, "Database error", http.StatusInternalServerError)
			return
		}

		// Assign a numeric value for each state for later sorting and Humanise timestamps
		enrichChecksData(checks)

		// We want to see the failures at the top
		sortByState(checks)

		responseWithJSON(w, http.StatusOK, checks)
	}
}

func sortByState(checks []model.ServiceStatus) {
	sort.Sort(byStateThenByName(checks))
}

func enrichChecksData(checks []model.ServiceStatus) {

	for idx, check := range checks {
		checks[idx].HumanisedCheckTime = humanize.Time(check.CheckTime)
		checks[idx].HumanisedStateSince = humanize.Time(check.StateSince)

		// Be lenient on those services which do not match the /health endpoint specification
		checks[idx].AggregatedState = strings.ToLower(check.AggregatedState)

		switch strings.ToLower(check.AggregatedState) {
		case "unhealthy":
			checks[idx].StatePriority = 1
		case "degraded":
			checks[idx].StatePriority = 2
		case "healthy":
			checks[idx].StatePriority = 3
		}
	}
}

func errorWithJSON(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintf(w, "{message: %q}", message)
}

func responseWithJSON(w http.ResponseWriter, successCode int, payload interface{}) {

	respBody, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		log.WithError(err).Errorf("json marshal error")
		errorWithJSON(w, "json marshal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(successCode)
	w.Write(respBody)
}

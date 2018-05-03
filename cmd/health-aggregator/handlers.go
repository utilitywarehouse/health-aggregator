package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type reloadHandler struct {
	discovery *serviceDiscovery
}

func newRouter(s *serviceDiscovery, mgoRepo *MongoRepository) *mux.Router {
	r := mux.NewRouter()

	reloader := reloadHandler{discovery: s}

	r.Handle("/reload", withRepoCopy(mgoRepo, reloader.reload)).Methods(http.MethodPost)
	r.Handle("/services", withRepoCopy(mgoRepo, getAllServices)).Methods(http.MethodGet)
	r.Handle("/namespaces", withRepoCopy(mgoRepo, getAllNamespaces)).Methods(http.MethodGet)
	r.Handle("/namespaces/{namespace}/services", withRepoCopy(mgoRepo, getServicesForNameSpace)).Methods(http.MethodGet)
	r.Handle("/namespaces/{namespace}/services/{service}/checks", withRepoCopy(mgoRepo, getAllChecksForService)).Methods(http.MethodGet)
	r.Handle("/namespaces/{namespace}/services/checks", withRepoCopy(mgoRepo, getLatestChecksForNamespace)).Methods(http.MethodGet)

	return r
}

func withRepoCopy(mgoRepo *MongoRepository, next func(mgoRepo *MongoRepository) http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repoCopy := mgoRepo.WithNewSession()
		defer repoCopy.Close()
		next(repoCopy).ServeHTTP(w, r)
	})
}

func (h reloadHandler) reload(mgoRepo *MongoRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, sChanOpen := (<-h.discovery.services)
		_, nChanOpen := (<-h.discovery.services)
		if sChanOpen || nChanOpen {
			errorWithJSON(w, "reload in progress - try again later", http.StatusServiceUnavailable)
			return
		}
		// Open new channels
		namespaces := make(chan namespace, 10)
		services := make(chan service, 10)

		// Assign new channels to
		h.discovery.services = services
		h.discovery.namespaces = namespaces
		go h.discovery.getClusterHealthcheckConfig()
		go upsertNamespaceConfigs(mgoRepo.WithNewSession(), namespaces, h.discovery.errors)
		go upsertServiceConfigs(mgoRepo.WithNewSession(), services, h.discovery.errors)
		responseWithJSON(w, http.StatusOK, map[string]string{"message": "ok"})
	}
}

func getAllNamespaces(mgoRepo *MongoRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ns, err := findAllNamespaces(mgoRepo)
		if err != nil {
			log.WithError(err).Errorf("database error - failed to get all namespaces")
			errorWithJSON(w, "Database error", http.StatusInternalServerError)
			return
		}

		responseWithJSON(w, http.StatusOK, ns)
	}
}

func getAllServices(mgoRepo *MongoRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svcs, err := findAllServices(mgoRepo)
		if err != nil {
			log.WithError(err).
				Errorf("database error - failed to get all services")
			errorWithJSON(w, "Database error", http.StatusInternalServerError)
			return
		}

		responseWithJSON(w, http.StatusOK, svcs)
	}
}

func getServicesForNameSpace(mgoRepo *MongoRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		ns := vars["namespace"]
		svcs, err := findAllServicesForNameSpace(mgoRepo, ns)
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

func getAllChecksForService(mgoRepo *MongoRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		n := vars["namespace"]
		s := vars["service"]

		checks, err := findAllChecksForService(mgoRepo, n, s)
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

func getLatestChecksForNamespace(mgoRepo *MongoRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		n := vars["namespace"]

		checks, err := findLatestChecksForNamespace(mgoRepo, n)
		if err != nil {
			log.WithField("namespace", n).
				WithError(err).
				Errorf("Database error")
			errorWithJSON(w, "Database error", http.StatusInternalServerError)
			return
		}

		if r.Header.Get("Accept") == "application/json" {
			responseWithJSON(w, http.StatusOK, checks)
		} else {
			var checkData templatedChecks
			checkData.Namespace = n
			checkData.Checks = checks

			cwd, cwdErr := os.Getwd()
			if cwdErr != nil {
				log.WithError(errors.Wrap(err, "failed to get current working directory"))
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(500)
				fmt.Fprint(w, "failed to get current working directory")
				return
			}

			tmpl, tmplErr := template.ParseFiles(filepath.Join(cwd, "../../templates/nschecks.html"))
			if tmplErr != nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(500)
				fmt.Fprint(w, "Failed to parse template")
				return
			}
			tmpl.Execute(w, checkData)
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

package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"github.com/utilitywarehouse/health-aggregator/internal/db"
	"github.com/utilitywarehouse/health-aggregator/internal/discovery"
)

// NewRouter returned a *mux.Router and sets up all required routes and handlers
func NewRouter(mgoRepo *db.MongoRepository, discoveryService *discovery.KubeDiscoveryService) *mux.Router {
	r := mux.NewRouter()

	r.Handle("/reload", reloader(mgoRepo, discoveryService)).Methods(http.MethodPost)

	return r
}

func withRepoCopy(mgoRepo *db.MongoRepository, next func(mgoRepo *db.MongoRepository) http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repoCopy := mgoRepo.WithNewSession()
		defer repoCopy.Close()
		next(repoCopy).ServeHTTP(w, r)
	})
}

func reloader(mgoRepo *db.MongoRepository, discoveryService *discovery.KubeDiscoveryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		go func(errs chan error) {
			for e := range errs {
				log.Printf("ERROR: %v", e)
			}
		}(discoveryService.Errors)

		servicesUpdater := db.NewK8sServicesConfigUpdater(discoveryService.Services, mgoRepo.WithNewSession())
		namespacesUpdater := db.NewK8sNamespacesConfigUpdater(discoveryService.Namespaces, mgoRepo.WithNewSession())

		go func() {
			namespacesUpdater.UpsertNamespaceConfigs()
		}()

		go func() {
			servicesUpdater.UpsertServiceConfigs()
		}()

		go func() {
			discoveryService.GetClusterHealthcheckConfig()
		}()

		responseWithJSON(w, http.StatusOK, map[string]string{"message": "ok"})
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

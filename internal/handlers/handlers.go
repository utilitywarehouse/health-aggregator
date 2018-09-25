package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"github.com/utilitywarehouse/health-aggregator/internal/db"
)

// NewRouter returned a *mux.Router and sets up all required routes and handlers
func NewRouter(reloadQueue chan uuid.UUID) *mux.Router {
	r := mux.NewRouter()

	r.Handle("/reload", reloader(reloadQueue)).Methods(http.MethodPost)

	return r
}

func withRepoCopy(mgoRepo *db.MongoRepository, next func(mgoRepo *db.MongoRepository) http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repoCopy := mgoRepo.WithNewSession()
		defer repoCopy.Close()
		next(repoCopy).ServeHTTP(w, r)
	})
}

func reloader(reloadQueue chan uuid.UUID) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reqID := uuid.Must(uuid.NewV4())
		reloadQueue <- uuid.Must(uuid.NewV4())
		responseWithJSON(w, http.StatusOK, map[string]string{"message": "reload request received for id " + reqID.String()})
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

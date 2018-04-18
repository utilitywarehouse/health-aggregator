package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

func getAllNamespaces(mgoRepo *MongoRepository) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		ns, err := findAllNamespaces(mgoRepo)
		if err != nil {
			log.WithError(err).
				Errorf("database error - failed to get all namespaces")
			errorWithJSON(w, "Database error", http.StatusInternalServerError)
			return
		}

		respBody, err := json.MarshalIndent(ns, "", "  ")
		if err != nil {
			log.WithError(err).
				Errorf("Json marshal error")
			errorWithJSON(w, "Json marshal error", http.StatusInternalServerError)
			return
		}

		responseWithJSON(w, respBody, http.StatusOK)
	}
}

func getAllServices(mgoRepo *MongoRepository) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		svcs, err := findAllServices(mgoRepo)
		if err != nil {
			log.WithError(err).
				Errorf("database error - failed to get all services")
			errorWithJSON(w, "Database error", http.StatusInternalServerError)
			return
		}

		respBody, err := json.MarshalIndent(svcs, "", "  ")
		if err != nil {
			log.WithError(err).
				Errorf("Json marshal error")
			errorWithJSON(w, "Json marshal error", http.StatusInternalServerError)
			return
		}

		responseWithJSON(w, respBody, http.StatusOK)
	}
}

func getServicesForNameSpace(mgoRepo *MongoRepository) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		ns := vars["namespace"]

		svcs, err := findAllServicesForNameSpace(mgoRepo, ns)
		if err != nil {
			if err == ErrorNoSuchNamespace {
				log.WithField("namespace", ns).
					WithError(err).
					Info()
				errorWithJSON(w, err.Error(), http.StatusNotFound)
				return
			}
			log.WithField("namespace", ns).
				WithError(err).
				Errorf("database error - failed to get services for namespace")
			errorWithJSON(w, "Database error", http.StatusInternalServerError)
			return
		}

		respBody, err := json.MarshalIndent(svcs, "", "  ")
		if err != nil {
			log.WithField("namespace", ns).
				WithError(err).
				Errorf("Json marshal error")
			errorWithJSON(w, "Json marshal error", http.StatusInternalServerError)
			return
		}

		responseWithJSON(w, respBody, http.StatusOK)
	}
}

func getChecksForService(mgoRepo *MongoRepository) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		s := vars["service"]

		checks, err := findAllChecksForService(mgoRepo, s)
		if err != nil {
			log.WithField("service", s).
				WithError(err).
				Errorf("Database error")
			errorWithJSON(w, "Database error", http.StatusInternalServerError)
			return
		}

		respBody, err := json.MarshalIndent(checks, "", "  ")
		if err != nil {
			log.WithField("service", s).
				WithError(err).
				Errorf("Json marshal error")
			errorWithJSON(w, "Json marshal error", http.StatusInternalServerError)
			return
		}

		responseWithJSON(w, respBody, http.StatusOK)
	}
}

func errorWithJSON(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintf(w, "{message: %q}", message)
}

func responseWithJSON(w http.ResponseWriter, json []byte, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	w.Write(json)
}

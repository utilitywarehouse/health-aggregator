package httpserver

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/handlers"
	log "github.com/sirupsen/logrus"
)

// New creates a HTTP server. Receives port, routes and write and read timeout
func New(port int, router http.Handler, writeTimeout int, readTimeout int, allowedCorsMethods handlers.CORSOption, allowedCorsOrigins handlers.CORSOption) *http.Server {
	return &http.Server{
		Handler: handlers.CORS(allowedCorsMethods, allowedCorsOrigins)(router),
		Addr:    fmt.Sprintf(":%d", port),
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: time.Duration(writeTimeout) * time.Second,
		ReadTimeout:  time.Duration(readTimeout) * time.Second,
	}
}

// Start starts an http server
func Start(server *http.Server) {
	log.Info("starting healthchecks api")
	if err := server.ListenAndServe(); err != nil {
		log.WithError(err).Fatal("Fatal error while running HTTP server")
	}
	log.Info("healthchecks api started with address " + server.Addr)
}

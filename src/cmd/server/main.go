package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Naufal-Pinasthika/IF3110_DB-Management-Platform/src/api"
)

func main() {
	// configure server port via Docker (:8080)
	addr := os.Getenv("APP_ADDR")

	// make server
	server := &http.Server{
		Addr: addr, 
		Handler: api.NewRouter(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout: 60 * time.Second,
	}
	log.Printf("HTTP server listening on %s", addr)

	// start server 
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}

}

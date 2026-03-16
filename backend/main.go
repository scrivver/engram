package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	pgHost := os.Getenv("PGHOST")
	amqpPort := os.Getenv("RABBITMQ_AMQP_PORT")

	log.Printf("Starting engram backend (PGHOST=%s, RABBITMQ_AMQP_PORT=%s)", pgHost, amqpPort)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	addr := ":8080"
	log.Printf("Listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

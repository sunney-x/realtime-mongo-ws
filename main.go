package main

import (
	"log"
	"net/http"
	"os"

	"github.com/sunney-x/realtime/realtime"
)

func main() {
	hub := realtime.New()
	port := ":8080"

	if portEnv := os.Getenv("PORT"); portEnv != "" {
		port = ":" + portEnv
	}
	log.Printf("Listening on %s", port)

	http.HandleFunc("/realtime", hub.Handler)
	log.Fatal(http.ListenAndServe(port, nil))
}

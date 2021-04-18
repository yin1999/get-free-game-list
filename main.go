package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	log.Print("starting server...\n")

	http.HandleFunc("/", handler)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

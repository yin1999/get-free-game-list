package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	os.Stderr.WriteString("starting server...\n")

	http.HandleFunc("/", handler)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

package main

import (
	"fmt"
	"net/http"
)

// Port wird festgelegt
const PORT = "3737"

func getHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

func getRoot(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "concept store")
}

func main() {
	// HandleFunctions einbinden
	http.HandleFunc("/health", getHealth)
	http.HandleFunc("/", getRoot)

	fmt.Printf("Health server unter http://localhost:%s/health\n", PORT)
	http.ListenAndServe(":"+PORT, nil)
}

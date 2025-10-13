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

var data = map[string]string{
	"1": "Antwort 1",
	"2": "Antwort 2",
}

func getAnswer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing query param: id", http.StatusBadRequest)
		return
	}

	val, ok := data[id]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, val)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", getHealth)
	mux.HandleFunc("/answer", getAnswer)
	mux.HandleFunc("/", getRoot)

	fmt.Printf("Health server unter http://localhost:%s/health\n", PORT)
	fmt.Printf("Dictionary unter http://localhost:%s/answer?id=x\n", PORT)
	http.ListenAndServe(":"+PORT, mux)
}

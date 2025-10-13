package main

import (
	"fmt"
	"net/http"
)

// Port wird festgelegt
const PORT = "3737"

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})
	fmt.Printf("Health server unter http://localhost:%s/health\n", PORT)
	http.ListenAndServe(":"+PORT, nil)
}

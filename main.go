package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"
)

// define PORT
const PORT = "3737"

func getHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")

	w.Header().Set("Cache-Control", "no-store")
}

func getRoot(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "concept store")
}

var Data = map[string]map[string]string{}

func LoadData(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&Data); err != nil {
		return fmt.Errorf("error decoding json: %w", err)
	}

	return nil
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

	val, ok := Data[id]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	format := r.URL.Query().Get("format")

	switch format {
	//FIXME: xml format for new data structure
	/*	case "xml":
		type AnswerXML struct {
			XMLName xml.Name `xml:"answer"`
			ID      string   `xml:"id"`
			Text    string   `xml:"text"`
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		xml.NewEncoder(w).Encode(AnswerXML{
			ID:   id,
			Text: val,
		})
	*/
	default: // JSON
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{ // TODO: make typesafe
			"id":     id,
			"answer": val,
		})
	}

}

func main() {
	err := LoadData("data.json")
	if err != nil {
		fmt.Printf("error reading files: %s\n", err)
		os.Exit(1)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", getHealth)
	mux.HandleFunc("/answer", getAnswer)
	mux.HandleFunc("/", getRoot)

	server := &http.Server{
		Addr:    ":" + PORT,
		Handler: mux,

		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,

		MaxHeaderBytes: 512 * 1024, // 512 KB
	}
	// channel for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	go func() {
		fmt.Printf("Server läuft unter http://localhost:%s\n", PORT)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Server error: %s\n", err)
		}
	}()

	// wait for shutdown signal
	<-stop
	fmt.Println("\nShutdown initiated...")

	// context for shutdown with 5s grace period
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		fmt.Printf("error shutting down: %s\n", err)
	} else {
		fmt.Printf("server successfully shut down.")
	}
}

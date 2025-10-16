package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"
)

// define PORT
const PORT = "3737"

func getHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

func getRoot(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "concept store")
}

type DataOutput struct {
	Unit        string `json:"Unit" xml:"Unit"`
	Value       string `json:"Value" xml:"Value"`
	Description string `json:"Description" xml:"Description"`
}

var Data = map[string]DataOutput{}

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
	case "xml":
		type AnswerXML struct {
			XMLName    xml.Name `xml:"answer"`
			ID         string   `xml:"id"`
			DataOutput `xml:",inline"`
		}

		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_ = xml.NewEncoder(w).Encode(AnswerXML{ID: id, DataOutput: val})

	default: // JSON
		type resp struct {
			ID     string     `json:"id"`
			Answer DataOutput `json:"answer"`
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp{ID: id, Answer: val})
	}
}

func main() {
	err := LoadData("data.json") // load file "data.json"
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
		fmt.Println("server successfully shut down.")
	}
}

package main

import (
	"context"
	"encoding/json"

	"encoding/xml"
	"fetchcdd"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	aasjsonization "github.com/aas-core-works/aas-core3.0-golang/jsonization"
	aastypes "github.com/aas-core-works/aas-core3.0-golang/types"
	aasxmlization "github.com/aas-core-works/aas-core3.0-golang/xmlization"
)

const FILENAME = fetchcdd.DataFilename
const MAINPAGE = "main_page.html"
const PORT = "3737"
const URLBASE = "http://localhost:"

var Data = map[string]aastypes.IConceptDescription{}

func LoadData(FILENAME string) error {
	file, err := os.Open(FILENAME)
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)

	var raw map[string]interface{}
	if err := decoder.Decode(&raw); err != nil {
		return fmt.Errorf("error decoding json: %w", err)
	}

	resultRaw, ok := raw["result"].([]interface{})
	if !ok {
		return fmt.Errorf("missing or invalid 'result' field")
	}

	for _, item := range resultRaw {
		cd, err := aasjsonization.ConceptDescriptionFromJsonable(item)
		if err != nil {
			fmt.Printf("error parsing ConceptDescription: %s\n", err)
			continue
		}
		Data[cd.ID()] = cd
	}

	fmt.Printf("Loaded %d ConceptDescriptions\n", len(Data))
	for id := range Data {
		fmt.Println("Available ID:", id)
	}

	return nil
}

func getHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

func getRoot(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, MAINPAGE)
}

func getAnswer(r *http.Request) (string, aastypes.IConceptDescription, int, error) {
	fmt.Println("getAnswer called")

	id := strings.TrimSpace(r.URL.Query().Get("id"))
	fmt.Println("Requested ID:", id)

	if id == "" {
		return "", &aastypes.ConceptDescription{}, http.StatusBadRequest, fmt.Errorf("missing query param: id")
	}

	if strings.HasPrefix(id, "0112/") {
		err := fetchcdd.GetIRDIfromCS(id)
		if err != nil {
			fmt.Printf("Error fetching IRDI: %s\n", err)
		} else {
			fmt.Println("fetchcdd call successful")
		}

		if err := LoadData(FILENAME); err != nil {
			fmt.Printf("Error reloading %s: %s\n", FILENAME, err)
		}
	}

	val, ok := Data[id]
	if !ok {
		fmt.Println("ID not found after fetch. Available IDs:")
		for k := range Data {
			fmt.Println("-", k)
		}
		return "", &aastypes.ConceptDescription{}, http.StatusNotFound, fmt.Errorf("not found")
	}

	return id, val, http.StatusOK, nil
}

func getJsonByPath(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/concept-store/")

	fullID := URLBASE + PORT + "/concept-store/" + id

	val, ok := Data[fullID]
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	jsonable, err := aasjsonization.ToJsonable(val)
	if err != nil {
		http.Error(w, "serialization error", http.StatusInternalServerError)
		return
	}
	if err := json.NewEncoder(w).Encode(jsonable); err != nil {
		http.Error(w, "json encode error", http.StatusInternalServerError)
	}

}

func getJson(w http.ResponseWriter, r *http.Request) {
	_, val, errCode, err := getAnswer(r)
	if err != nil {
		http.Error(w, err.Error(), errCode)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	jsonable, err := aasjsonization.ToJsonable(val)
	if err != nil {
		http.Error(w, "serialization error", http.StatusInternalServerError)
		return
	}
	if err := json.NewEncoder(w).Encode(jsonable); err != nil {
		http.Error(w, "json encode error", http.StatusInternalServerError)
	}
}

func getXml(w http.ResponseWriter, r *http.Request) {
	_, val, errCode, err := getAnswer(r)
	if err != nil {
		http.Error(w, err.Error(), errCode)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	w.Write([]byte(xml.Header))
	if err := aasxmlization.Marshal(xml.NewEncoder(w), val, true); err != nil {
		http.Error(w, "XML serialization error", http.StatusInternalServerError)
		return
	}

}

func main() {
	err := LoadData(FILENAME)
	if err != nil {
		fmt.Printf("error reading files: %s\n", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", getHealth)
	mux.HandleFunc("/json", getJson)
	mux.HandleFunc("/xml", getXml)
	mux.HandleFunc("/", getRoot)
	mux.HandleFunc("/concept-store/", getJsonByPath)

	server := &http.Server{
		Addr:           ":" + PORT,
		Handler:        mux,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   10 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 512 * 1024,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	go func() {
		fmt.Printf("server address: http://localhost:%s\n", PORT)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Server error: %s\n", err)
		}
	}()

	<-stop
	fmt.Println("\nShutdown initiated...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		fmt.Printf("error shutting down: %s\n", err)
	} else {
		fmt.Println("server successfully shut down.")
	}
}

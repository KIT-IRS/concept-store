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
)

const PORT = "3737"

// Datenstrukturen
type LangString struct {
	Language string `json:"language" xml:"language"`
	Text     string `json:"text" xml:"text"`
}

type DataSpecificationContent struct {
	ModelType     string       `json:"modelType" xml:"modelType"`
	DataType      string       `json:"dataType" xml:"dataType"`
	Definition    []LangString `json:"definition" xml:"definition"`
	PreferredName []LangString `json:"preferredName" xml:"preferredName"`
	ShortName     []LangString `json:"shortName,omitempty" xml:"shortName,omitempty"`
	Unit          string       `json:"unit" xml:"unit"`
}

type Key struct {
	Type  string `json:"type" xml:"type"`
	Value string `json:"value" xml:"value"`
}

type DataSpecification struct {
	Keys []Key  `json:"keys" xml:"keys"`
	Type string `json:"type" xml:"type"`
}

type EmbeddedDataSpecification struct {
	DataSpecification        DataSpecification        `json:"dataSpecification" xml:"dataSpecification"`
	DataSpecificationContent DataSpecificationContent `json:"dataSpecificationContent" xml:"dataSpecificationContent"`
}

type ConceptDescription struct {
	ModelType                  string                      `json:"modelType" xml:"modelType"`
	EmbeddedDataSpecifications []EmbeddedDataSpecification `json:"embeddedDataSpecifications" xml:"embeddedDataSpecifications"`
	ID                         string                      `json:"id" xml:"id"`
	IDShort                    string                      `json:"idShort" xml:"idShort"`
}

var Data = map[string]ConceptDescription{}

func LoadData(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}
	defer file.Close()

	var wrapper struct {
		Result []ConceptDescription `json:"result"`
	}

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&wrapper); err != nil {
		return fmt.Errorf("error decoding json: %w", err)
	}

	for _, cd := range wrapper.Result {
		Data[cd.ID] = cd
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
	http.ServeFile(w, r, "main_page.html")
}

func getAnswer(r *http.Request) (string, ConceptDescription, int, error) {
	fmt.Println("getAnswer called")

	id := strings.TrimSpace(r.URL.Query().Get("id"))
	fmt.Println("Requested ID:", id)

	if id == "" {
		return "", ConceptDescription{}, http.StatusBadRequest, fmt.Errorf("missing query param: id")
	}

	if strings.HasPrefix(id, "0112/2//") {
		err := fetchcdd.GetIRDIfromCS(id)
		if err != nil {
			fmt.Printf("Error fetching IRDI: %s\n", err)
		} else {
			fmt.Println("fetchcdd call successful")
		}

		if err := LoadData("data.json"); err != nil {
			fmt.Printf("Error reloading data.json: %s\n", err)
		}
	}

	val, ok := Data[id]
	if !ok {
		fmt.Println("ID not found after fetch. Available IDs:")
		for k := range Data {
			fmt.Println("-", k)
		}
		return "", ConceptDescription{}, http.StatusNotFound, fmt.Errorf("not found")
	}

	return id, val, http.StatusOK, nil
}

func getJsonByPath(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/concept-store/")

	fullID := "http://localhost:3737/concept-store/" + id

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
	_ = json.NewEncoder(w).Encode(val)
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
	_ = json.NewEncoder(w).Encode(val)
}

func getXml(w http.ResponseWriter, r *http.Request) {
	id, val, errCode, err := getAnswer(r)
	if err != nil {
		http.Error(w, err.Error(), errCode)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	type AnswerXML struct {
		XMLName            xml.Name `xml:"answer"`
		ID                 string   `xml:"id"`
		ConceptDescription `xml:",inline"`
	}
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_ = xml.NewEncoder(w).Encode(AnswerXML{ID: id, ConceptDescription: val})
}

func main() {
	err := LoadData("data.json")
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

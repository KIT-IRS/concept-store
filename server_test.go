package main

import (
	"encoding/json"
	"fetchcdd"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	aasjsonization "github.com/aas-core-works/aas-core3.0-golang/jsonization"
	aastypes "github.com/aas-core-works/aas-core3.0-golang/types"
)

type DataFile struct {
	PagingMetadata map[string]any                 `json:"paging_metadata"`
	Result         []aastypes.IConceptDescription `json:"result"`
}

// make a test http server
func newTestServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", getHealth)
	mux.HandleFunc("/json", getJson)
	mux.HandleFunc("/xml", getXml)
	mux.HandleFunc("/", getRoot)
	mux.HandleFunc("/concept-store/", getJsonByPath)
	return httptest.NewServer(mux)
}
func sendRequest(t *testing.T, method, UrlEnding string) *http.Response {
	t.Helper()
	ts := newTestServer()
	defer ts.Close()

	URL := ts.URL + UrlEnding
	req, err := http.NewRequest(method, URL, nil)
	if err != nil {
		t.Fatalf("error during %v %v without id: %v", method, URL, err)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("error with %v %v %v", method, UrlEnding, err)
	}
	return resp
}
func ptr(s string) *string {
	return &s
}

func getTestData() map[string]aastypes.IConceptDescription {
	cd := &aastypes.ConceptDescription{}
	cd.SetID("11")
	cd.SetIDShort(ptr("Voltage"))

	return map[string]aastypes.IConceptDescription{
		"11": cd,
	}
}

func TestGetHealth(t *testing.T) {

	request, _ := http.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	getHealth(response, request)

	got := response.Body.String()
	want := "OK"

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	res := response.Result()
	defer res.Body.Close()

	// test status code
	if res.StatusCode != http.StatusOK {
		t.Errorf("Statuscode: got %d, want %d", res.StatusCode, http.StatusOK)
	}

}

func TestLoadData(t *testing.T) {
	testDataMap := getTestData()

	var jsonables []interface{}
	for _, v := range testDataMap {
		j, err := aasjsonization.ToJsonable(v)
		if err != nil {
			t.Fatalf("error converting to jsonable: %v", err)
		}
		jsonables = append(jsonables, j)
	}

	wrapper := map[string]interface{}{
		"result": jsonables,
	}

	tmpFile, err := os.CreateTemp("", "testdata*.json")
	if err != nil {
		t.Fatalf("error creating temporary file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	encoder := json.NewEncoder(tmpFile)
	if err := encoder.Encode(wrapper); err != nil {
		t.Fatalf("error writing testdata to file: %v", err)
	}
	tmpFile.Close()

	err = LoadData(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadData function gave an error: %v", err)
	}

	got, ok := Data["11"]
	if !ok {
		t.Fatalf("error could not find ID")
	}
	if got.IDShort() == nil || *got.IDShort() != "Voltage" {
		t.Errorf("error loaded IDShort does not match: %v", got.IDShort())
	}
}

func TestGetJson(t *testing.T) {
	Data = getTestData()

	t.Run("Success", func(t *testing.T) {
		resp := sendRequest(t, http.MethodGet, "/json?id=11")
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Status %d, expected %d", resp.StatusCode, http.StatusOK)
		}
		if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
			t.Fatalf("Wrong Content-Type")
		}

		var raw interface{}
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			t.Fatalf("error decoding JSON: %v", err)
		}

		cd, err := aasjsonization.ConceptDescriptionFromJsonable(raw)
		if err != nil {
			t.Fatalf("error converting to ConceptDescription: %v", err)
		}

		if cd.ID() != "11" {
			t.Errorf("ID=%q, expected %q", cd.ID(), "11")
		}
		if cd.IDShort() == nil || *cd.IDShort() != "Voltage" {
			t.Errorf("IDShort=%q, expected %q", *cd.IDShort(), "Voltage")
		}
	})
	t.Run("MissingID", func(t *testing.T) {
		resp := sendRequest(t, http.MethodGet, "/json")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("InvalidID", func(t *testing.T) {
		resp := sendRequest(t, http.MethodGet, "/json?id=999")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("WrongMethod", func(t *testing.T) {
		resp := sendRequest(t, http.MethodPut, "/json?id=11")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("Expected 405, got %d", resp.StatusCode)
		}
	})
}

func TestGetXml(t *testing.T) {
	Data = getTestData()

	t.Run("Success", func(t *testing.T) {
		resp := sendRequest(t, http.MethodGet, "/xml?id=11")
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Status %d, expected %d", resp.StatusCode, http.StatusOK)
		}
		if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/xml") {
			t.Fatalf("Wrong Content-Type")
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		xmlContent := string(bodyBytes)

		if !strings.Contains(xmlContent, "<id>11</id>") {
			t.Errorf("Missing <id> tag: %s", xmlContent)
		}
		if !strings.Contains(xmlContent, "<idShort>Voltage</idShort>") {
			t.Errorf("Missing <idShort> tag: %s", xmlContent)
		}

	})

	t.Run("MissingID", func(t *testing.T) {
		resp := sendRequest(t, http.MethodGet, "/xml")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("InvalidID", func(t *testing.T) {
		resp := sendRequest(t, http.MethodGet, "/xml?id=999")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("WrongMethod", func(t *testing.T) {
		resp := sendRequest(t, http.MethodPut, "/xml?id=11")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("Expected 405, got %d", resp.StatusCode)
		}
	})
}
func TestGetJsonByPath(t *testing.T) {

	cd := &aastypes.ConceptDescription{}
	cd.SetID("11")
	cd.SetIDShort(ptr("Voltage"))

	Data = map[string]aastypes.IConceptDescription{
		"http://localhost:3737/concept-store/11": cd,
	}

	ts := newTestServer()
	defer ts.Close()

	t.Run("Success", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/concept-store/11")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, expected %d", resp.StatusCode, http.StatusOK)
		}
		if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
			t.Errorf("Wrong Content-Type: %s", resp.Header.Get("Content-Type"))
		}

		var raw interface{}
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			t.Fatalf("error decoding JSON: %v", err)
		}

		cd, err := aasjsonization.ConceptDescriptionFromJsonable(raw)
		if err != nil {
			t.Fatalf("error converting to ConceptDescription: %v", err)
		}

		if cd.ID() != "11" {
			t.Errorf("ID=%q, expected %q", cd.ID(), "11")
		}
		if cd.IDShort() == nil || *cd.IDShort() != "Voltage" {
			t.Errorf("IDShort=%q, expected %q", *cd.IDShort(), "Voltage")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/concept-store/999")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("StatusCode = %d, expected %d", resp.StatusCode, http.StatusNotFound)
		}
	})

	t.Run("WrongMethod", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/concept-store/11", nil)
		if err != nil {
			t.Fatalf("Request creation failed: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("StatusCode = %d, expected %d", resp.StatusCode, http.StatusMethodNotAllowed)
		}
	})
}
func TestFetchcdd(t *testing.T) {
	irdi := "0112/2///61360_4#AAA398#001"

	tmpDir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	defer os.Chdir(cwd)

	if err := fetchcdd.GetIRDIfromCS(irdi); err != nil {
		t.Fatalf("GetIRDIfromCS returned error: %v", err)
	}

	exists, err := fetchcdd.IdExistsInDataFile(irdi, fetchcdd.DataFilename)
	if err != nil {
		t.Fatalf("idExistsInDataFile failed: %v", err)
	}
	if !exists {
		t.Fatalf("expected id %s to exist in %s after GetIRDIfromCS", irdi, fetchcdd.DataFilename)
	}

	df, err := fetchcdd.ReadDataFile(fetchcdd.DataFilename)
	if err != nil {
		t.Fatalf("readDataFile failed: %v", err)
	}

	var cleaned []aastypes.IConceptDescription
	for _, cd := range df.Result {
		if cd.ID() == irdi {
			continue
		}
		cleaned = append(cleaned, cd)
	}
	df.Result = cleaned

	if err := fetchcdd.WriteDataFileAtomic(fetchcdd.DataFilename, df); err != nil {
		t.Fatalf("writeDataFileAtomic failed during cleanup: %v", err)
	}

	exists, err = fetchcdd.IdExistsInDataFile(irdi, fetchcdd.DataFilename)
	if err != nil {
		t.Fatalf("idExistsInDataFile failed after cleanup: %v", err)
	}
	if exists {
		t.Fatalf("expected id %s to be removed from %s after cleanup", irdi, fetchcdd.DataFilename)
	}
}

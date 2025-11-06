package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

type DataFile struct {
	PagingMetadata map[string]any       `json:"paging_metadata"`
	Result         []ConceptDescription `json:"result"`
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
func getTestData() map[string]ConceptDescription {
	return map[string]ConceptDescription{
		"11": {
			ModelType: "ConceptDescription",
			ID:        "11",
			IDShort:   "Voltage",
			EmbeddedDataSpecifications: []EmbeddedDataSpecification{
				{
					DataSpecification: DataSpecification{
						Type: "DataSpecificationIEC61360",
						Keys: []Key{
							{Type: "GlobalReference", Value: "some-value"},
						},
					},
					DataSpecificationContent: DataSpecificationContent{
						ModelType: "DataSpecificationIEC61360",
						DataType:  "REAL_MEASURE",
						Unit:      "Volt",
						PreferredName: []LangString{
							{Language: "en", Text: "Voltage"},
						},
						Definition: []LangString{
							{Language: "en", Text: "Electric potential difference"},
						},
					},
				},
			},
		},
	}
}
func TestGetHealth(t *testing.T) {
	t.Run("returns 200 and OK", func(t *testing.T) {
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

	})

}
func TestLoadData(t *testing.T) {
	testDataMap := getTestData()

	var testDataSlice []ConceptDescription
	for _, v := range testDataMap {
		testDataSlice = append(testDataSlice, v)
	}

	wrapper := struct {
		Result []ConceptDescription `json:"result"`
	}{
		Result: testDataSlice,
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
	if got.IDShort != "Voltage" {
		t.Errorf("error loaded IDShort does not match: %v", got.IDShort)
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

		var result ConceptDescription
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("error decoding JSON: %v", err)
		}

		if result.ID != "11" {
			t.Errorf("ID=%q, expected %q", result.ID, "11")
		}
		if result.IDShort != "Voltage" {
			t.Errorf("IDShort=%q, expected %q", result.IDShort, "Voltage")
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
		if !strings.Contains(xmlContent, "<unit>Volt</unit>") {
			t.Errorf("Missing <unit> tag: %s", xmlContent)
		}
		if !strings.Contains(xmlContent, "<text>Electric potential difference</text>") {
			t.Errorf("Missing <definition> text: %s", xmlContent)
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

	Data = map[string]ConceptDescription{
		"http://localhost:3737/concept-store/11": {
			ModelType: "ConceptDescription",
			ID:        "11",
			IDShort:   "Voltage",
		},
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

		var result ConceptDescription
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("JSON decode error: %v", err)
		}
		if result.ID != "11" {
			t.Errorf("ID = %q, expected %q", result.ID, "11")
		}
		if result.IDShort != "Voltage" {
			t.Errorf("IDShort = %q, expected %q", result.IDShort, "Voltage")
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

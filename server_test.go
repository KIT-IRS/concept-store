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

// make a test http server
func newTestServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", getHealth)
	mux.HandleFunc("/json", getJson)
	mux.HandleFunc("/xml", getXml)
	mux.HandleFunc("/", getRoot)
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
func getTestData() map[string]DataOutput {
	return map[string]DataOutput{
		"11": {
			Unit:        "Volt",
			Value:       "5",
			Description: "example1",
		},
	}
}

// TestGetAnswerJSON_Success tests whether a valid json is put out
// does not test data.json

// happy path


func TestLoadData(t *testing.T) {
	testData := getTestData()

	// create temporary file
	tmpFile, err := os.CreateTemp("", "testdata*.json")
	if err != nil {
		t.Fatalf("error creating temporary file: %v", err)
	}
	defer os.Remove(tmpFile.Name()) // remove temporary file after test

	// write Data to file
	encoder := json.NewEncoder(tmpFile)
	if err := encoder.Encode(testData); err != nil {
		t.Fatalf("error writing testdata to file: %v", err)
	}
	tmpFile.Close()

	// load temporary file
	err = LoadData(tmpFile.Name())
	if err != nil {
		t.Fatalf("LoadData function gave an error: %v", err)
	}

	// check loaded Data
	got, ok := Data["11"]
	if !ok {
		t.Fatalf("error could not find ID")
	}
	if got.Unit != "Volt" {
		t.Errorf("error loaded Unit does not match: %v", got)
	}
	if got.Value != "5" {
		t.Errorf("error loaded Value does not match: %v", got)
	}
	if got.Description != "example1" {
		t.Errorf("error loaded Description does not match: %v", got)
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

		var result struct {
			ID     string     `json:"id"`
			Answer DataOutput `json:"answer"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("error decoding JSON: %v", err)
		}

		if result.ID != "11" {
			t.Errorf("ID=%q, expected %q", result.ID, "11")
		}
		if result.Answer.Unit != "Volt" {
			t.Errorf("Answer.Unit=%q, expected %q", result.Answer.Unit, "Volt")
		}
		if result.Answer.Value != "5" {
			t.Errorf("Answer.Value=%q, expected %q", result.Answer.Value, "5")
		}
		if result.Answer.Description != "example1" {
			t.Errorf("Answer.Description=%q, expected %q", result.Answer.Description, "example1")
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
		if !strings.Contains(xmlContent, "<Unit>Volt</Unit>") {
			t.Errorf("XML-Contents contain unexpected <Unit>: %s", xmlContent)
		}
		if !strings.Contains(xmlContent, "<Value>5</Value>") {
			t.Errorf("XML-Contents contain unexpected <Value>: %s", xmlContent)
		}
		if !strings.Contains(xmlContent, "<Description>example1</Description>") {
			t.Errorf("XML-Contents contain unexpected <Description>: %s", xmlContent)
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

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		res := response.Result() // echtes *http.Response Objekt
		defer res.Body.Close()

		// prüfe status code
		if res.StatusCode != http.StatusOK {
			t.Errorf("Statuscode: got %d, want %d", res.StatusCode, http.StatusOK)
		}

	})

}

// make a test http server
func newTestServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", getHealth)
	mux.HandleFunc("/answer", getAnswer)
	mux.HandleFunc("/", getRoot)
	return httptest.NewServer(mux)
}

// TestGetAnswerJSON_Success tests whether a valid json is put out
// TODO: test that tests reading data file
func TestGetAnswerJSON_Success(t *testing.T) {
	// Data struct for testing
	Data = map[string]string{
		"11": "Test Antwort",
	}

	ts := newTestServer()
	defer ts.Close()
	// issues GET to URL
	resp, err := http.Get(ts.URL + "/answer?id=11")
	if err != nil {
		t.Fatalf("error during GET /answer: %v", err)
	}
	defer resp.Body.Close()
	// check status code
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Status %d, expected %d", resp.StatusCode, http.StatusOK)
	}
	// get header, check if json format
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type %q, expected application/json", ct)
	}

	var result struct {
		ID     string `json:"id"`
		Answer string `json:"answer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("error decoding JSON: %v", err)
	}

	if result.ID != "11" {
		t.Errorf("ID=%q, expected %q", result.ID, "11")
	}
	if result.Answer != "Test Antwort" {
		t.Errorf("Answer=%q, expected %q", result.Answer, "Test Antwort")
	}
}

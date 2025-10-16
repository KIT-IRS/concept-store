package main

import (
	"encoding/json"
	"io"
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

// TODO: test that tests reading data file

// happy path
func TestGetAnswerJSON_Success(t *testing.T) {
	// Data struct for testing
	Data = getTestData()

	resp := sendRequest(t, http.MethodGet, "/answer?id=11")
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

}
func TestGetAnswerXML_Success(t *testing.T) {
	Data = getTestData()
	resp := sendRequest(t, http.MethodGet, "/answer?id=11&format=xml")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Status %d, expected %d", resp.StatusCode, http.StatusOK)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/xml") {
		t.Fatalf("Content-Type %q, expected application/xml", ct)
	}

	// check xml contents

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("error reading response body: %v", err)
	}

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
}

// test error for missing ID
func TestGetAnswer_MissingID(t *testing.T) {
	resp := sendRequest(t, http.MethodGet, "/answer")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Statuscode: got %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

}
func TestGetAnswer_InvalidID(t *testing.T) {
	resp := sendRequest(t, http.MethodGet, "/answer?id=3333")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Statuscode: got %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

}
func TestGetAnswer_WrongMethod(t *testing.T) {
	resp := sendRequest(t, http.MethodPut, "/answer?id=11")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Statuscode: got %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

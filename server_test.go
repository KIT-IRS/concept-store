package main

import (
	"net/http"
	"net/http/httptest"
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

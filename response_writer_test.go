package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusMethod(t *testing.T) {
	res := httptest.NewRecorder()

	w := ResponseWriter(&responseWriter{rw: res})
	w.Status(http.StatusInternalServerError)

	got := res.Code
	want := http.StatusInternalServerError

	if got != want {
		t.Errorf("got status %d, but want %d", got, want)
	}
}

func TestSetMethod(t *testing.T) {
	res := httptest.NewRecorder()

	w := ResponseWriter(&responseWriter{rw: res})
	w.Set("Content-Type", "application/json")

	got := res.Header().Get("Content-Type")
	want := "application/json"

	if got != want {
		t.Errorf("got %s, but want %s", got, want)
	}
}

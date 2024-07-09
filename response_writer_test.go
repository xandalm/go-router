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

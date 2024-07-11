package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetStatusMethod(t *testing.T) {
	res := httptest.NewRecorder()

	w := ResponseWriter(&responseWriter{rw: res})

	want := http.StatusInternalServerError
	w.SetStatus(want)

	got := res.Code

	if got != want {
		t.Errorf("got status %d, but want %d", got, want)
	}
}

func TestSetHeaderMethod(t *testing.T) {
	res := httptest.NewRecorder()

	w := ResponseWriter(&responseWriter{rw: res})

	want := "application/json"
	w.SetHeader("Content-Type", want)

	got := res.Header().Get("Content-Type")

	if got != want {
		t.Errorf("got %q, but want %q", got, want)
	}
}

func TestWriteMethod(t *testing.T) {
	t.Run("writes string and read from response", func(t *testing.T) {
		res := httptest.NewRecorder()

		w := ResponseWriter(&responseWriter{rw: res})

		want := "some data"
		w.Write(want)

		data, _ := io.ReadAll(res.Body)
		got := string(data)

		if got != want {
			t.Errorf("got %q, but want %q", got, want)
		}
	})
}

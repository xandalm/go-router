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

type stubResponseWriter struct{}

func (rw *stubResponseWriter) SetHeader(key string, value string) {}

func (rw *stubResponseWriter) SetStatus(code int) {}

func (rw *stubResponseWriter) Write(v any) {
	panic(PanicMsgWritingError)
}

func TestWriteMethod(t *testing.T) {

	cases := []struct {
		value any
		want  string
	}{
		{"some data", "some data"},
		{1, "1"},
		{1.2, "1.2"},
		{-2, "-2"},
	}

	for _, c := range cases {
		t.Run("writes value as string and read from response", func(t *testing.T) {
			res := httptest.NewRecorder()

			w := ResponseWriter(&responseWriter{rw: res})

			w.Write(c.value)

			data, _ := io.ReadAll(res.Body)
			got := string(data)

			if got != c.want {
				t.Errorf("got %q, but want %q", got, c.want)
			}
		})
	}
	t.Run("panics on writing error", func(t *testing.T) {
		w := &stubResponseWriter{}
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("didn't panic")
			}
			if r != PanicMsgWritingError {
				t.Errorf("panics %s but want %s", r, PanicMsgWritingError)
			}
		}()
		w.Write("")
	})
}

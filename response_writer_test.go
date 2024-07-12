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
}

func TestWriteJSONMethod(t *testing.T) {
	cases := []struct {
		value any
		want  string
	}{
		{
			value: "text",
			want:  `"text"`,
		},
		{
			value: 1,
			want:  "1",
		},
		{
			value: 1.2,
			want:  "1.2",
		},
		{
			value: []int{},
			want:  "[]",
		},
		{
			value: []int{1, 2},
			want:  "[1,2]",
		},
		{
			value: struct {
				FieldOne int
				FieldTwo string
			}{1, "any"},
			want: `{"FieldOne":1,"FieldTwo":"any"}`,
		},
		{
			value: struct {
				fieldOne int
				fieldTwo string
			}{1, "any"},
			want: `{}`,
		},
		{
			value: struct {
				Field int `json:"field"`
			}{1},
			want: `{"field":1}`,
		},
	}

	for _, c := range cases {
		t.Run("writes value following json format and read from response", func(t *testing.T) {
			res := httptest.NewRecorder()

			w := ResponseWriter(&responseWriter{rw: res})

			w.WriteJSON(c.value)

			data, _ := io.ReadAll(res.Body)
			got := string(data)

			if got != c.want {
				t.Errorf("got %q, but want %q", got, c.want)
			}
		})
	}
}

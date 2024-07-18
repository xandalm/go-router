package router

import (
	"bytes"
	"io"
	"math"
	"math/bits"
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

func TestSendMethod(t *testing.T) {
	type tcase struct {
		desc  string
		value any
		want  []byte
	}

	createPosIntBytes := func() []byte {
		if bits.UintSize == 32 {
			return []byte{0, 0, 0, 1}
		}
		return []byte{0, 0, 0, 0, 0, 0, 0, 1}
	}

	createNegIntBytes := func() []byte {
		if bits.UintSize == 32 {
			return []byte{255, 255, 255, 255}
		}
		return []byte{255, 255, 255, 255, 255, 255, 255, 255}
	}

	createUintBytes := func() []byte {
		if bits.UintSize == 32 {
			return []byte{0, 0, 0, 1}
		}
		return []byte{0, 0, 0, 0, 0, 0, 0, 1}
	}

	pi8 := new(int8)
	ppi8 := &pi8

	cases := []tcase{
		{"sends string bytes", "some data", []byte("some data")},
		{"sends int bytes", int(1), createPosIntBytes()},
		{"sends negative int bytes", int(-1), createNegIntBytes()},
		{"sends int8 bytes", int8(1), []byte{1}},
		{"sends int16 bytes", int16(1), []byte{0, 1}},
		{"sends int32 bytes", int32(1), []byte{0, 0, 0, 1}},
		{"sends int64 bytes", int64(1), []byte{0, 0, 0, 0, 0, 0, 0, 1}},
		{"sends uint bytes", uint(1), createUintBytes()},
		{"sends uint8 bytes", uint8(1), []byte{1}},
		{"sends uint16 bytes", uint16(1), []byte{0, 1}},
		{"sends uint32 bytes", uint32(1), []byte{0, 0, 0, 1}},
		{"sends uint64 bytes", uint64(1), []byte{0, 0, 0, 0, 0, 0, 0, 1}},
		{"sends *int8 (referenced int8) bytes", pi8, []byte{0}},
		{"sends **int8 (ref. ref. int8) bytes", ppi8, []byte{0}},
		{"sends float32 bytes", float32(1.0), u32b(math.Float32bits(1.0))},
		{"sends float64 bytes", float64(1.0), u64b(math.Float64bits(1.0))},
		{"sends rune bytes", 'A', []byte{0, 0, 0, 65}}, // alright, rune is int32 alias
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			res := httptest.NewRecorder()

			w := ResponseWriter(&responseWriter{rw: res})

			err := w.Send(c.value)
			assertNoError(t, err)

			got, _ := io.ReadAll(res.Body)

			if !bytes.Equal(got, c.want) {
				t.Errorf("got %q, but want %q", got, c.want)
			}
		})
	}
}

func TestSendStringMethod(t *testing.T) {

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
		t.Run("sends value as string and read from response", func(t *testing.T) {
			res := httptest.NewRecorder()

			w := ResponseWriter(&responseWriter{rw: res})

			w.SendString(c.value)

			data, _ := io.ReadAll(res.Body)
			got := string(data)

			if got != c.want {
				t.Errorf("got %q, but want %q", got, c.want)
			}
		})
	}
}

func TestSendJSONMethod(t *testing.T) {
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
		t.Run("sends value following json format and read from response", func(t *testing.T) {
			res := httptest.NewRecorder()

			w := ResponseWriter(&responseWriter{rw: res})

			w.SendJSON(c.value)

			data, _ := io.ReadAll(res.Body)
			got := string(data)

			if got != c.want {
				t.Errorf("got %q, but want %q", got, c.want)
			}
		})
	}
}

package router

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestRequest(t *testing.T) {

	t.Run("returns params", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, newDummyURI("/path/something"), nil)
		params := Params{
			"param": "something",
		}

		request := &Request{
			params,
			req,
		}

		if !reflect.DeepEqual(request.URL, req.URL) {
			t.Errorf("got url %v, but want %v", request.URL, req.URL)
		}

		if !reflect.DeepEqual(request.Params(), params) {
			t.Errorf("got params %v, but want %v", request.Params(), params)
		}
	})

	t.Run("returns empty params", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, newDummyURI("/path"), nil)
		params := Params{}

		request := &Request{
			Request: req,
		}

		if !reflect.DeepEqual(request.URL, req.URL) {
			t.Errorf("got url %v, but want %v", request.URL, req.URL)
		}

		if !reflect.DeepEqual(params, request.Params()) {
			t.Errorf("got params %#v, but want %#v", request.Params(), params)
		}
	})
}

func TestParseBodyInto(t *testing.T) {

	t.Run("panic if not give pointer", func(t *testing.T) {
		request := newRequest(http.MethodPost, newDummyURI("/words"), "science")

		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
		}()

		var bucket string
		request.ParseBodyInto(bucket)
	})

	t.Run("panic if give a nil pointer", func(t *testing.T) {
		request := newRequest(http.MethodPost, newDummyURI("/words"), "science")

		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
		}()

		var bucket *string
		request.ParseBodyInto(bucket)
	})

	t.Run("parses body into string", func(t *testing.T) {
		request := newRequest(http.MethodPost, newDummyURI("/words"), "router")

		var bucket string
		err := request.ParseBodyInto(&bucket)

		assertNoError(t, err)

		if bucket != "router" {
			t.Errorf(`got %q, but want "router"`, bucket)
		}
	})

	t.Run("parses body even for re-typed string", func(t *testing.T) {
		type S string

		request := newRequest(http.MethodPost, newDummyURI("/words"), "science")

		var bucket S
		err := request.ParseBodyInto(&bucket)

		assertNoError(t, err)

		if bucket != "science" {
			t.Errorf(`got %s, but want "science"`, bucket)
		}
	})

	t.Run("parses body into integer", func(t *testing.T) {
		request := newRequest(http.MethodPut, newDummyURI("/add"), "5")

		var bucket int
		err := request.ParseBodyInto(&bucket)

		assertNoError(t, err)

		if bucket != 5 {
			t.Errorf(`got %d, but want 5`, bucket)
		}
	})

	t.Run("parses body even for re-typed int", func(t *testing.T) {
		type I int

		request := newRequest(http.MethodPut, newDummyURI("/add"), "5")

		var bucket I
		err := request.ParseBodyInto(&bucket)

		assertNoError(t, err)

		if bucket != 5 {
			t.Errorf(`got %d, but want 5`, bucket)
		}
	})

	t.Run("returns error for incompatible int", func(*testing.T) {
		request := newRequest(http.MethodPost, newDummyURI("/sub"), "a")

		var bucket int
		err := request.ParseBodyInto(&bucket)
		if err != ErrUnsupportedInt {
			t.Errorf("got error %v, but want %v", err, ErrUnsupportedInt)
		}
	})

	t.Run("parses body into float", func(t *testing.T) {
		request := newRequest(http.MethodPut, newDummyURI("/add"), "3.14")

		var bucket float64
		err := request.ParseBodyInto(&bucket)

		assertNoError(t, err)

		if bucket != 3.14 {
			t.Errorf(`got %f, but want 3.14`, bucket)
		}
	})

	t.Run("parses json body into struct", func(t *testing.T) {
		type Person struct {
			Id   int
			Name string
		}

		request := newRequest(http.MethodPost, newDummyURI("/persons"), `{"Id": 1, "Name": "Alex"}`)

		var got Person
		want := Person{1, "Alex"}
		err := request.ParseBodyInto(&got)

		assertNoError(t, err)

		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v, but want %+v", got, want)
		}
	})

	t.Run("cause nothing to read error", func(t *testing.T) {

		req, _ := http.NewRequest(http.MethodPost, newDummyURI("/add"), nil)
		request := &Request{Request: req}
		var got int
		err := request.ParseBodyInto(&got)

		if err != ErrNilBody {
			t.Errorf("got error %v but want %v", got, ErrNilBody)
		}
	})
}

func newRequest(method, url, body string) *Request {
	r, _ := http.NewRequest(method, url, strings.NewReader(body))
	return &Request{
		Request: r,
	}
}

func assertNoError(t testing.TB, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("expected no error, %v", err)
	}
}

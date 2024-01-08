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

	t.Run("panic if not give pointer type to ParseBodyInto", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, newDummyURI("/words"), strings.NewReader("router"))

		request := &Request{
			Request: req,
		}

		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
		}()

		var bucket string
		request.ParseBodyInto(bucket)
	})

	t.Run("parses body into string", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, newDummyURI("/words"), strings.NewReader("router"))

		request := &Request{
			Request: req,
		}

		var bucket string
		request.ParseBodyInto(&bucket)

		if bucket != "router" {
			t.Errorf(`got %q, but want "router"`, bucket)
		}
	})

	t.Run("parses body into integer", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, newDummyURI("/add"), strings.NewReader("5"))

		request := &Request{
			Request: req,
		}

		var bucket int
		request.ParseBodyInto(&bucket)

		if bucket != 5 {
			t.Errorf(`got %d, but want 5`, bucket)
		}
	})

	t.Run("parses body even for re-typed string", func(t *testing.T) {
		type S string

		req, _ := http.NewRequest(http.MethodPost, newDummyURI("/words"), strings.NewReader("science"))

		request := &Request{
			Request: req,
		}

		var bucket S
		request.ParseBodyInto(&bucket)

		if bucket != "science" {
			t.Errorf(`got %s, but want "science"`, bucket)
		}
	})

	t.Run("parses body even for re-typed int", func(t *testing.T) {
		type I int

		req, _ := http.NewRequest(http.MethodPost, newDummyURI("/add"), strings.NewReader("5"))

		request := &Request{
			Request: req,
		}

		var bucket I
		request.ParseBodyInto(&bucket)

		if bucket != 5 {
			t.Errorf(`got %d, but want 5`, bucket)
		}
	})

	t.Run("parses json body into struct", func(t *testing.T) {
		type Person struct {
			Id   int
			Name string
		}

		req, _ := http.NewRequest(http.MethodPost, newDummyURI("/add"), strings.NewReader(`{"Id": 1, "Name": "Alex"}`))

		request := &Request{
			Request: req,
		}

		var got Person
		request.ParseBodyInto(&got)
		want := Person{1, "Alex"}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v, but want %+v", got, want)
		}
	})
}

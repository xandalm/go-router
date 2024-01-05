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
}

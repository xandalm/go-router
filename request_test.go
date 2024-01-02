package router

import (
	"net/http"
	"reflect"
	"testing"
)

func TestRequest(t *testing.T) {

	t.Run("returns params", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, newDummyURI("/path/something"), nil)
		params := map[string]string{
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
		params := map[string]string{}

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

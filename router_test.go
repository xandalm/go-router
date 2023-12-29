package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

type dummyRouteHandler struct{}

func (h *dummyRouteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

}

var dummyHandler = &dummyRouteHandler{}
var dummyHandlerFunc = func(w http.ResponseWriter, r *http.Request) {
}

func Test_Register(t *testing.T) {

	t.Run("panic on empty pattern", func(t *testing.T) {
		router := &Router{}

		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
		}()
		router.register("", dummyHandler, MethodAll)
	})

	t.Run("panic on nil handler", func(t *testing.T) {
		router := &Router{}

		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
		}()

		router.register("/path", nil, MethodAll)
	})

	t.Run("panic on re-register same pattern and method", func(t *testing.T) {
		router := &Router{}

		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
		}()

		router.register("/path", dummyHandler, MethodAll)
		router.register("/path", dummyHandler, MethodAll)
	})

	cases := []struct {
		path   string
		method string
	}{
		{"/users", MethodAll},
		{"/api/users", MethodAll},
		{"/users", MethodGet},
		{"/users", MethodPost},
		{"/users", MethodPut},
		{"/users", MethodDelete},
	}

	router := &Router{}

	for _, c := range cases {
		t.Run(fmt.Sprintf(`add %q to %s`, c.path, c.method), func(t *testing.T) {

			router.register(c.path, dummyHandler, c.method)

			assertRegistered(t, router, c.path)

			e := router.m[c.path]
			assertHandler(t, e.mh[c.method], dummyHandler)
		})
	}
}

func Test_RegisterFunc(t *testing.T) {

	t.Run("panic on nil handler", func(t *testing.T) {
		router := &Router{}

		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
		}()

		router.registerFunc("/path", nil, MethodAll)
	})

	cases := []struct {
		path   string
		method string
	}{
		{"/users", MethodAll},
		{"/users", MethodGet},
		{"/users", MethodPost},
		{"/users", MethodPut},
		{"/users", MethodDelete},
	}

	router := &Router{}

	for _, c := range cases {
		t.Run(fmt.Sprintf(`add %q to %s`, c.path, c.method), func(t *testing.T) {

			router.registerFunc(c.path, dummyHandlerFunc, c.method)

			assertRegistered(t, router, c.path)

			e := router.m[c.path]
			assertHandlerFunc(t, e.mh[c.method], RouteHandlerFunc(dummyHandlerFunc))
		})
	}
}

type MockRouterHandler struct {
	OnHandleFunc func(http.ResponseWriter, *http.Request)
}

func (h *MockRouterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.OnHandleFunc(w, r)
}

func newDummyURI(path string) string {
	return "http://site.com" + path
}

type routeCase struct {
	path    string
	handler *MockRouterHandler
	tests   []uriTest
}

type uriTest struct {
	uri    string
	method string
	status int
	body   string
}

func TestUse(t *testing.T) {

	cases := []routeCase{
		{
			path: "/users",
			handler: &MockRouterHandler{
				OnHandleFunc: func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprint(w, r.Method)
				},
			},
			tests: []uriTest{
				{newDummyURI("/users"), http.MethodGet, http.StatusOK, ""},
			},
		},
	}

	router := &Router{}
	for _, c := range cases {
		t.Run(fmt.Sprintf("add %q pattern", c.path), func(t *testing.T) {

			router.Use(c.path, c.handler)

			for _, tt := range c.tests {
				t.Run(fmt.Sprintf("request %s on %q", tt.method, tt.uri), func(t *testing.T) {
					request, _ := http.NewRequest(tt.method, tt.uri, nil)
					response := httptest.NewRecorder()

					router.ServeHTTP(response, request)

					if response.Code != tt.status {
						t.Errorf("got status %q, but want %q", response.Code, tt.status)
					}
				})
			}
		})
	}

}

func TestGet(t *testing.T) {

	cases := []routeCase{
		{
			path: "/products",
			handler: &MockRouterHandler{
				OnHandleFunc: func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprint(w, `[{"Name": "Tea"}, {"Name": "Cup Noodle"}]`)
				},
			},
			tests: []uriTest{
				{newDummyURI("/products"), http.MethodGet, http.StatusOK, `[{"Name": "Tea"}, {"Name": "Cup Noodle"}]`},
				{newDummyURI("/products"), http.MethodPost, http.StatusNotFound, ""},
			},
		},
	}

	router := &Router{}

	for _, c := range cases {
		t.Run(fmt.Sprintf("add path %q", c.path), func(t *testing.T) {

			router.Get(c.path, c.handler)

			for _, tt := range c.tests {
				t.Run(fmt.Sprintf("request %s on %q", tt.method, tt.uri), func(t *testing.T) {
					request, _ := http.NewRequest(tt.method, tt.uri, nil)
					response := httptest.NewRecorder()

					router.ServeHTTP(response, request)

					if response.Code != tt.status {
						t.Errorf("got status %q, but want %q", response.Code, tt.status)
					}

					body := response.Body.String()
					if body != tt.body {
						t.Errorf("got body %q, but want %q", body, tt.body)
					}
				})
			}
		})
	}
}

func assertRegistered(t testing.TB, router *Router, path string) {
	t.Helper()

	if _, ok := router.m[path]; !ok {
		t.Fatal("not registered the pattern")
	}
}

func assertHandler(t testing.TB, got, want RouteHandler) {
	t.Helper()

	if got != want {
		t.Errorf("got handler %v, but want %v", got, want)
	}
}

func assertHandlerFunc(t testing.TB, got RouteHandler, want func(http.ResponseWriter, *http.Request)) {
	t.Helper()

	w := RouteHandlerFunc(want)
	if !reflect.DeepEqual(reflect.ValueOf(got), reflect.ValueOf(w)) {
		t.Errorf("got handler %#v, but want %#v", got, w)
	}
}

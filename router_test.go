package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
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

	userRE := regexp.MustCompile(`^\/users$`)

	cases := []struct {
		pattern string
		re      *regexp.Regexp
		method  string
	}{
		{"/users", userRE, MethodAll},
		{"/api/users", regexp.MustCompile(`^\/api\/users$`), MethodAll},
		{"/users", userRE, MethodGet},
		{"/users", userRE, MethodPost},
		{"/users", userRE, MethodPut},
		{"/users", userRE, MethodDelete},
		{"/users/{id}", regexp.MustCompile(`^\/users\/(?P<id>[^\/]+)$`), MethodGet},
	}

	router := &Router{}

	for _, c := range cases {
		t.Run(fmt.Sprintf(`add %q to %s`, c.pattern, c.method), func(t *testing.T) {

			router.register(c.pattern, dummyHandler, c.method)

			assertRegistered(t, router, c.pattern)

			e := router.m[c.pattern]
			assertHandler(t, e.mh[c.method], dummyHandler)

			if !reflect.DeepEqual(c.re, e.re) {
				t.Errorf("got regexp %q, but want %q", e.re, c.re)
			}
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

func TestHandler(t *testing.T) {
	t.Run("returns pattern, handler", func(t *testing.T) {
		router := NewRouter()

		pattern := "/path"
		path := "/path"

		router.Use(pattern, dummyHandler)

		request, _ := http.NewRequest(http.MethodGet, newDummyURI(path), nil)

		var pat string
		var h RouteHandler
		pat, h, _ = router.Handler(request)

		if pat != pattern {
			t.Errorf("got pattern %q, but want %q", pat, pattern)
		}
		assertHandler(t, dummyHandler, h)
	})

	cases := []struct {
		desc            string
		pattern         string
		path            string
		expectedPattern string
		expectedHandler RouteHandler
		expectedParams  map[string]string
	}{
		{
			"returns pattern, handler and id equal to 1 in params",
			"/users/{id}",
			"/users/1",
			"/users/{id}",
			dummyHandler,
			map[string]string{
				"id": "1",
			},
		},
		{
			"returns pattern, handler and id equal to 6dbd2 in params",
			"/users/{id}",
			"/users/6dbd2",
			"/users/{id}",
			dummyHandler,
			map[string]string{
				"id": "6dbd2",
			},
		},
		{
			"returns pattern, handler and id equal to d033fdc6-dbd2-427c-b18c-a41aa6449d75 in params",
			"/users/{id}",
			"/users/d033fdc6-dbd2-427c-b18c-a41aa6449d75",
			"/users/{id}",
			dummyHandler,
			map[string]string{
				"id": "d033fdc6-dbd2-427c-b18c-a41aa6449d75",
			},
		},
		{
			"returns pattern, handler and id equal to {id} in params",
			"/users/{id}",
			"/users/{id}",
			"/users/{id}",
			dummyHandler,
			map[string]string{
				"id": "{id}",
			},
		},
		{
			"returns empty pattern, not found handler and nil params",
			"/users/{id}",
			"/users/",
			"",
			NotFoundHandler,
			nil,
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			router := NewRouter()

			router.Use(c.pattern, dummyHandler)

			request, _ := http.NewRequest(http.MethodGet, newDummyURI(c.path), nil)

			var pat string
			var h RouteHandler
			var params map[string]string
			pat, h, params = router.Handler(request)

			if pat != c.expectedPattern {
				t.Errorf("got pattern %q, but want %q", pat, c.expectedPattern)
			}
			assertHandler(t, h, c.expectedHandler)
			if !reflect.DeepEqual(c.expectedParams, params) {
				t.Errorf("got params %v, but want %v", params, c.expectedParams)
			}
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

	for _, c := range cases {
		t.Run(fmt.Sprintf("add %q pattern", c.path), func(t *testing.T) {
			router := &Router{}

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
				{newDummyURI("/products"), http.MethodPut, http.StatusNotFound, ""},
				{newDummyURI("/products"), http.MethodDelete, http.StatusNotFound, ""},
			},
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("add path %q", c.path), func(t *testing.T) {
			router := &Router{}

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

func TestPost(t *testing.T) {

	cases := []routeCase{
		{
			path: "/products",
			handler: &MockRouterHandler{
				OnHandleFunc: func(w http.ResponseWriter, r *http.Request) {},
			},
			tests: []uriTest{
				{newDummyURI("/products"), http.MethodPost, http.StatusOK, ""},
				{newDummyURI("/products"), http.MethodGet, http.StatusNotFound, ""},
				{newDummyURI("/products"), http.MethodPut, http.StatusNotFound, ""},
				{newDummyURI("/products"), http.MethodDelete, http.StatusNotFound, ""},
			},
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("add path %q", c.path), func(t *testing.T) {
			router := &Router{}

			router.Post(c.path, c.handler)

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

func TestPut(t *testing.T) {

	cases := []routeCase{
		{
			path: "/products",
			handler: &MockRouterHandler{
				OnHandleFunc: func(w http.ResponseWriter, r *http.Request) {},
			},
			tests: []uriTest{
				{newDummyURI("/products"), http.MethodPut, http.StatusOK, ""},
				{newDummyURI("/products"), http.MethodGet, http.StatusNotFound, ""},
				{newDummyURI("/products"), http.MethodPost, http.StatusNotFound, ""},
				{newDummyURI("/products"), http.MethodDelete, http.StatusNotFound, ""},
			},
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("add path %q", c.path), func(t *testing.T) {
			router := &Router{}

			router.Put(c.path, c.handler)

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

func TestDelete(t *testing.T) {

	cases := []routeCase{
		{
			path: "/products",
			handler: &MockRouterHandler{
				OnHandleFunc: func(w http.ResponseWriter, r *http.Request) {},
			},
			tests: []uriTest{
				{newDummyURI("/products"), http.MethodDelete, http.StatusOK, ""},
				{newDummyURI("/products"), http.MethodGet, http.StatusNotFound, ""},
				{newDummyURI("/products"), http.MethodPut, http.StatusNotFound, ""},
				{newDummyURI("/products"), http.MethodPost, http.StatusNotFound, ""},
			},
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("add path %q", c.path), func(t *testing.T) {
			router := &Router{}

			router.Delete(c.path, c.handler)

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

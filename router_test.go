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

func (h *dummyRouteHandler) ServeHTTP(w ResponseWriter, r *Request) {

}

var dummyHandler = &dummyRouteHandler{}
var dummyHandlerFunc = func(w ResponseWriter, r *Request) {
}

func Test_register(t *testing.T) {

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

func Test_registerFunc(t *testing.T) {

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
		uri             string
		expectedPattern string
		expectedHandler RouteHandler
		expectedParams  Params
	}{
		{
			"returns pattern, handler and id equal to 1 in params",
			"/users/{id}",
			newDummyURI("/users/1"),
			"/users/{id}",
			dummyHandler,
			Params{
				"id": "1",
			},
		},
		{
			"returns pattern, handler and id equal to 6dbd2 in params",
			"/users/{id}",
			newDummyURI("/users/6dbd2"),
			"/users/{id}",
			dummyHandler,
			Params{
				"id": "6dbd2",
			},
		},
		{
			"returns pattern, handler and id equal to d033fdc6-dbd2-427c-b18c-a41aa6449d75 in params",
			"/users/{id}",
			newDummyURI("/users/d033fdc6-dbd2-427c-b18c-a41aa6449d75"),
			"/users/{id}",
			dummyHandler,
			Params{
				"id": "d033fdc6-dbd2-427c-b18c-a41aa6449d75",
			},
		},
		{
			"returns pattern, handler and id equal to {id} in params",
			"/users/{id}",
			newDummyURI("/users/{id}"),
			"/users/{id}",
			dummyHandler,
			Params{
				"id": "{id}",
			},
		},
		{
			"returns empty pattern, not found handler and nil params",
			"/users/{id}",
			newDummyURI("/users/"),
			"",
			NotFoundHandler,
			nil,
		},
		{
			"returns pattern, handler and empty params",
			"site.com/users",
			"http://site.com/users",
			"site.com/users",
			dummyHandler,
			Params{},
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			router := NewRouter()

			router.Use(c.pattern, dummyHandler)

			request, _ := http.NewRequest(http.MethodGet, c.uri, nil)

			var pat string
			var h RouteHandler
			var params Params
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
	lastParams   Params
	OnHandleFunc func(ResponseWriter, *Request)
}

func (h *MockRouterHandler) ServeHTTP(w ResponseWriter, r *Request) {
	h.lastParams = r.Params()
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
	params Params
	status int
	body   string
}

func TestUse(t *testing.T) {

	cases := []routeCase{
		{
			path: "/users",
			handler: &MockRouterHandler{
				OnHandleFunc: func(w ResponseWriter, r *Request) {
				},
			},
			tests: []uriTest{
				{newDummyURI("/users"), http.MethodGet, Params{}, http.StatusOK, ""},
			},
		},
		{
			path: "/users/{id}",
			handler: &MockRouterHandler{
				OnHandleFunc: func(w ResponseWriter, r *Request) {
				},
			},
			tests: []uriTest{
				{newDummyURI("/users/13"), http.MethodGet, Params{"id": "13"}, http.StatusOK, ""},
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

					assertStatus(t, response, tt.status)

					if !reflect.DeepEqual(c.handler.lastParams, tt.params) {
						t.Errorf("got params %#v, but want %#v", c.handler.lastParams, tt.params)
					}

					assertBody(t, response, tt.body)
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
				OnHandleFunc: func(w ResponseWriter, r *Request) {
					fmt.Fprint(w, `[{"Name": "Tea"}, {"Name": "Cup Noodle"}]`)
				},
			},
			tests: []uriTest{
				{newDummyURI("/products"), http.MethodGet, Params{}, http.StatusOK, `[{"Name": "Tea"}, {"Name": "Cup Noodle"}]`},
				{newDummyURI("/products"), http.MethodPost, Params{}, http.StatusNotFound, ""},
				{newDummyURI("/products"), http.MethodPut, Params{}, http.StatusNotFound, ""},
				{newDummyURI("/products"), http.MethodDelete, Params{}, http.StatusNotFound, ""},
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

					assertStatus(t, response, tt.status)

					assertBody(t, response, tt.body)
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
				OnHandleFunc: func(w ResponseWriter, r *Request) {},
			},
			tests: []uriTest{
				{newDummyURI("/products"), http.MethodPost, Params{}, http.StatusOK, ""},
				{newDummyURI("/products"), http.MethodGet, Params{}, http.StatusNotFound, ""},
				{newDummyURI("/products"), http.MethodPut, Params{}, http.StatusNotFound, ""},
				{newDummyURI("/products"), http.MethodDelete, Params{}, http.StatusNotFound, ""},
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

					assertStatus(t, response, tt.status)

					assertBody(t, response, tt.body)
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
				OnHandleFunc: func(w ResponseWriter, r *Request) {},
			},
			tests: []uriTest{
				{newDummyURI("/products"), http.MethodPut, Params{}, http.StatusOK, ""},
				{newDummyURI("/products"), http.MethodGet, Params{}, http.StatusNotFound, ""},
				{newDummyURI("/products"), http.MethodPost, Params{}, http.StatusNotFound, ""},
				{newDummyURI("/products"), http.MethodDelete, Params{}, http.StatusNotFound, ""},
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

					assertStatus(t, response, tt.status)

					assertBody(t, response, tt.body)
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
				OnHandleFunc: func(w ResponseWriter, r *Request) {},
			},
			tests: []uriTest{
				{newDummyURI("/products"), http.MethodDelete, Params{}, http.StatusOK, ""},
				{newDummyURI("/products"), http.MethodGet, Params{}, http.StatusNotFound, ""},
				{newDummyURI("/products"), http.MethodPut, Params{}, http.StatusNotFound, ""},
				{newDummyURI("/products"), http.MethodPost, Params{}, http.StatusNotFound, ""},
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

					assertStatus(t, response, tt.status)

					assertBody(t, response, tt.body)
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

func assertHandlerFunc(t testing.TB, got RouteHandler, want func(ResponseWriter, *Request)) {
	t.Helper()

	w := RouteHandlerFunc(want)
	if !reflect.DeepEqual(reflect.ValueOf(got), reflect.ValueOf(w)) {
		t.Errorf("got handler %#v, but want %#v", got, w)
	}
}

func assertStatus(t testing.TB, response *httptest.ResponseRecorder, status int) {
	t.Helper()

	if response.Code != status {
		t.Errorf("got status %q, but want %q", response.Code, status)
	}
}

func assertBody(t testing.TB, response *httptest.ResponseRecorder, want string) {
	t.Helper()

	got := response.Body.String()
	if got != want {
		t.Errorf("got body %q, but want %q", got, want)
	}
}

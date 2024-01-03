package router

import (
	"fmt"
	"io"
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

	cases := []struct {
		pattern             string
		uri                 string
		expectedPattern     string
		expectedHandlerType reflect.Type
		expectedParams      Params
	}{
		{
			"/path",
			newDummyURI("/path"),
			"/path",
			reflect.TypeOf(dummyHandler),
			Params{},
		},
		{
			"/users/{id}",
			newDummyURI("/users/1"),
			"/users/{id}",
			reflect.TypeOf(dummyHandler),
			Params{
				"id": "1",
			},
		},
		{
			"/users/{id}",
			newDummyURI("/users/6dbd2"),
			"/users/{id}",
			reflect.TypeOf(dummyHandler),
			Params{
				"id": "6dbd2",
			},
		},
		{
			"/users/{id}",
			newDummyURI("/users/d033fdc6-dbd2-427c-b18c-a41aa6449d75"),
			"/users/{id}",
			reflect.TypeOf(dummyHandler),
			Params{
				"id": "d033fdc6-dbd2-427c-b18c-a41aa6449d75",
			},
		},
		{
			"/users/{id}",
			newDummyURI("/users/{id}"),
			"/users/{id}",
			reflect.TypeOf(dummyHandler),
			Params{
				"id": "{id}",
			},
		},
		{
			"/users/{id}",
			newDummyURI("/users/"),
			"",
			reflect.TypeOf(NotFoundHandler),
			nil,
		},
		{
			"site.com/users",
			"http://site.com/users",
			"site.com/users",
			reflect.TypeOf(dummyHandler),
			Params{},
		},
		{
			"/users/",
			newDummyURI("/users"),
			"/users/",
			reflect.TypeOf(&redirectHandler{}),
			nil,
		},
		{
			"/api/v1/partners",
			newDummyURI("/api/v1/products/../partners"),
			"/api/v1/partners",
			reflect.TypeOf(&redirectHandler{}),
			nil,
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("when listen to %q and request %q", c.pattern, c.uri), func(t *testing.T) {
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
			assertHandlerType(t, c.expectedHandlerType, h)
			assertParams(t, params, c.expectedParams)
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
	uri            string
	method         string
	body           io.Reader
	expectedParams Params
	expectedStatus int
	expectedBody   string
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
				{newDummyURI("/users"), http.MethodGet, nil, Params{}, http.StatusOK, ""},
			},
		},
		{
			path: "/users/{id}",
			handler: &MockRouterHandler{
				OnHandleFunc: func(w ResponseWriter, r *Request) {
				},
			},
			tests: []uriTest{
				{newDummyURI("/users/13"), http.MethodGet, nil, Params{"id": "13"}, http.StatusOK, ""},
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

					assertStatus(t, response, tt.expectedStatus)

					assertParams(t, c.handler.lastParams, tt.expectedParams)

					assertBody(t, response, tt.expectedBody)
				})
			}
		})
	}

}

func TestGet(t *testing.T) {

	t.Run(`router with only "/products" on GET`, func(t *testing.T) {
		router := NewRouter()

		router.Get("/products", dummyHandler)

		cases := []uriTest{
			{newDummyURI("/products"), http.MethodGet, nil, Params{}, http.StatusOK, ""},
			{newDummyURI("/products"), http.MethodPost, nil, Params{}, http.StatusNotFound, ""},
			{newDummyURI("/products"), http.MethodPut, nil, Params{}, http.StatusNotFound, ""},
			{newDummyURI("/products"), http.MethodDelete, nil, Params{}, http.StatusNotFound, ""},
		}

		for _, c := range cases {
			t.Run(fmt.Sprintf("returns %d for %s %q", c.expectedStatus, c.method, c.uri), func(t *testing.T) {
				request, _ := http.NewRequest(c.method, c.uri, nil)
				response := httptest.NewRecorder()

				router.ServeHTTP(response, request)

				assertStatus(t, response, c.expectedStatus)
			})
		}
	})
}

func TestPost(t *testing.T) {

	t.Run(`router with only "/products" on POST`, func(t *testing.T) {
		router := NewRouter()

		router.Post("/products", dummyHandler)

		cases := []uriTest{
			{newDummyURI("/products"), http.MethodPost, nil, Params{}, http.StatusOK, ""},
			{newDummyURI("/products"), http.MethodGet, nil, Params{}, http.StatusNotFound, ""},
			{newDummyURI("/products"), http.MethodPut, nil, Params{}, http.StatusNotFound, ""},
			{newDummyURI("/products"), http.MethodDelete, nil, Params{}, http.StatusNotFound, ""},
		}

		for _, c := range cases {
			t.Run(fmt.Sprintf("returns %d for %s %q", c.expectedStatus, c.method, c.uri), func(t *testing.T) {
				request, _ := http.NewRequest(c.method, c.uri, nil)
				response := httptest.NewRecorder()

				router.ServeHTTP(response, request)

				assertStatus(t, response, c.expectedStatus)
			})
		}
	})
}

func TestPut(t *testing.T) {

	t.Run(`router with only "/products" on PUT`, func(t *testing.T) {
		router := NewRouter()

		router.Put("/products", dummyHandler)

		cases := []uriTest{
			{newDummyURI("/products"), http.MethodPut, nil, Params{}, http.StatusOK, ""},
			{newDummyURI("/products"), http.MethodGet, nil, Params{}, http.StatusNotFound, ""},
			{newDummyURI("/products"), http.MethodPost, nil, Params{}, http.StatusNotFound, ""},
			{newDummyURI("/products"), http.MethodDelete, nil, Params{}, http.StatusNotFound, ""},
		}

		for _, c := range cases {
			t.Run(fmt.Sprintf("returns %d for %s %q", c.expectedStatus, c.method, c.uri), func(t *testing.T) {
				request, _ := http.NewRequest(c.method, c.uri, nil)
				response := httptest.NewRecorder()

				router.ServeHTTP(response, request)

				assertStatus(t, response, c.expectedStatus)
			})
		}
	})
}

func TestDelete(t *testing.T) {

	t.Run(`router with only "/products" on DELETE`, func(t *testing.T) {
		router := NewRouter()

		router.Delete("/products", dummyHandler)

		cases := []uriTest{
			{newDummyURI("/products"), http.MethodDelete, nil, Params{}, http.StatusOK, ""},
			{newDummyURI("/products"), http.MethodGet, nil, Params{}, http.StatusNotFound, ""},
			{newDummyURI("/products"), http.MethodPut, nil, Params{}, http.StatusNotFound, ""},
			{newDummyURI("/products"), http.MethodPost, nil, Params{}, http.StatusNotFound, ""},
		}

		for _, c := range cases {
			t.Run(fmt.Sprintf("returns %d for %s %q", c.expectedStatus, c.method, c.uri), func(t *testing.T) {
				request, _ := http.NewRequest(c.method, c.uri, nil)
				response := httptest.NewRecorder()

				router.ServeHTTP(response, request)

				assertStatus(t, response, c.expectedStatus)
			})
		}
	})
}

func TestRouter(t *testing.T) {

	router := NewRouter()
	t.Run(`handle to GET "/api/users" after add "/api/users" on GET`, func(t *testing.T) {
		handler := &MockRouterHandler{
			OnHandleFunc: func(w ResponseWriter, r *Request) {
				fmt.Fprint(w, `[]`)
			},
		}
		router.Get("/api/users", handler)

		request, _ := http.NewRequest(http.MethodGet, newDummyURI("/api/users"), nil)
		response := httptest.NewRecorder()

		router.ServeHTTP(response, request)

		assertStatus(t, response, http.StatusOK)
		assertParams(t, handler.lastParams, Params{})
		assertBody(t, response, `[]`)
	})
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
		t.Errorf("got status %d, but want %d", response.Code, status)
	}
}

func assertBody(t testing.TB, response *httptest.ResponseRecorder, want string) {
	t.Helper()

	got := response.Body.String()
	if got != want {
		t.Errorf("got body %q, but want %q", got, want)
	}
}

func assertHandlerType(t testing.TB, want reflect.Type, got RouteHandler) {
	t.Helper()

	tp := reflect.TypeOf(got)
	if tp != want {
		t.Errorf("got handler type %v, but want %v", tp, want)
	}
}

func assertParams(t testing.TB, got, want Params) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got params %#v, but want %#v", got, want)
	}
}

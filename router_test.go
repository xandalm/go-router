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

type dummyHandler struct{}

func (h *dummyHandler) ServeHTTP(w ResponseWriter, r *Request) {
}

var vDummyHandler = &dummyHandler{}
var vDummyHandlerFunc = func(w ResponseWriter, r *Request) {
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
		router.register("", vDummyHandler, MethodAll)
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

		router.register("/path", vDummyHandler, MethodAll)
		router.register("/path", vDummyHandler, MethodAll)
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

			router.register(c.pattern, vDummyHandler, c.method)

			assertRegistered(t, router, c.pattern)

			e := router.m[c.pattern]
			assertHandler(t, e.mh[c.method], vDummyHandler)

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
		pattern string
		method  string
	}{
		{"/users", MethodAll},
		{"/users", MethodGet},
		{"/users", MethodPost},
		{"/users", MethodPut},
		{"/users", MethodDelete},
	}

	router := &Router{}

	for _, c := range cases {
		t.Run(fmt.Sprintf(`add %q to %s`, c.pattern, c.method), func(t *testing.T) {

			router.registerFunc(c.pattern, vDummyHandlerFunc, c.method)

			assertRegistered(t, router, c.pattern)

			checkHandlerFunc(t, router, c.pattern, c.method, vDummyHandlerFunc)
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
			reflect.TypeOf(vDummyHandler),
			Params{},
		},
		{
			"/users/{id}",
			newDummyURI("/users/1"),
			"/users/{id}",
			reflect.TypeOf(vDummyHandler),
			Params{
				"id": "1",
			},
		},
		{
			"/users/{id}",
			newDummyURI("/users/6dbd2"),
			"/users/{id}",
			reflect.TypeOf(vDummyHandler),
			Params{
				"id": "6dbd2",
			},
		},
		{
			"/users/{id}",
			newDummyURI("/users/d033fdc6-dbd2-427c-b18c-a41aa6449d75"),
			"/users/{id}",
			reflect.TypeOf(vDummyHandler),
			Params{
				"id": "d033fdc6-dbd2-427c-b18c-a41aa6449d75",
			},
		},
		{
			"/users/{id}",
			newDummyURI("/users/{id}"),
			"/users/{id}",
			reflect.TypeOf(vDummyHandler),
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
			reflect.TypeOf(vDummyHandler),
			Params{},
		},
		{
			"site.com/users",
			"http://site.com:3000/users",
			"site.com/users",
			reflect.TypeOf(vDummyHandler),
			Params{},
		},
		{
			"site.com/users/",
			"http://site.com/users",
			"site.com/users/",
			reflect.TypeOf(&redirectHandler{}),
			nil,
		},
		{
			"/users/",
			newDummyURI("/users"),
			"/users/",
			reflect.TypeOf(&redirectHandler{}),
			nil,
		},
		{
			"/users",
			newDummyURI("/users/"),
			"/users",
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

			router.UsePattern(c.pattern, vDummyHandler)

			request, _ := http.NewRequest(http.MethodGet, c.uri, nil)

			var pat string
			var h Handler
			var params Params
			h, pat, params = router.Handler(request)

			if pat != c.expectedPattern {
				t.Errorf("got pattern %q, but want %q", pat, c.expectedPattern)
			}
			assertHandlerType(t, c.expectedHandlerType, h)
			assertParams(t, params, c.expectedParams)
		})
	}

	t.Run(`distinguish "/users" from "/users/" when both were added`, func(t *testing.T) {
		router := NewRouter()

		handlerOne := &MockRouterHandler{
			OnHandleFunc: func(w ResponseWriter, r *Request) {
			},
		}
		handlerTwo := &MockRouterHandler{
			OnHandleFunc: func(w ResponseWriter, r *Request) {
			},
		}

		router.UsePattern("/users/", handlerOne)
		router.UsePattern("/users", handlerTwo)

		cases := []struct {
			path    string
			pattern string
			handler *MockRouterHandler
			params  Params
		}{
			{
				path:    "/users/",
				pattern: "/users/",
				handler: handlerOne,
				params:  Params{},
			},
			{
				path:    "/users",
				pattern: "/users",
				handler: handlerTwo,
				params:  Params{},
			},
		}

		for _, c := range cases {
			request, _ := http.NewRequest(http.MethodGet, newDummyURI(c.path), nil)

			var pat string
			var h Handler
			var params Params

			h, pat, params = router.Handler(request)

			if pat != c.pattern {
				t.Errorf("got pattern %q, but want %q", pat, c.pattern)
			}

			assertHandler(t, h, c.handler)

			assertParams(t, params, c.params)
		}
	})
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

			router.UsePattern(c.path, c.handler)

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

		router.Get("/products", vDummyHandler)

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

		router.Post("/products", vDummyHandler)

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

		router.Put("/products", vDummyHandler)

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

		router.Delete("/products", vDummyHandler)

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

	t.Run(`handle to GET "/admin/users" after add "/admin/users" on GET`, func(t *testing.T) {
		handler := &MockRouterHandler{
			OnHandleFunc: func(w ResponseWriter, r *Request) {
				fmt.Fprint(w, `[]`)
			},
		}
		router.Get("/admin/users", handler)

		request, _ := http.NewRequest(http.MethodGet, newDummyURI("/admin/users"), nil)
		response := httptest.NewRecorder()

		router.ServeHTTP(response, request)

		assertStatus(t, response, http.StatusOK)
		assertParams(t, handler.lastParams, Params{})
		assertBody(t, response, `[]`)
	})
}

func BenchmarkRouterMath(b *testing.B) {
	r := NewRouter()
	r.UsePattern("/", vDummyHandler)
	r.UsePattern("/index", vDummyHandler)
	r.UsePattern("/home", vDummyHandler)
	r.UsePattern("/about", vDummyHandler)
	r.UsePattern("/contact", vDummyHandler)
	r.UsePattern("/robots.txt", vDummyHandler)
	r.UsePattern("/products/", vDummyHandler)
	r.UsePattern("/products/{id}", vDummyHandler)
	r.UsePattern("/products/{id}/image.jpg", vDummyHandler)
	r.UsePattern("/admin", vDummyHandler)
	r.UsePattern("/admin/products/", vDummyHandler)
	r.UsePattern("/admin/products/create", vDummyHandler)
	r.UsePattern("/admin/products/update", vDummyHandler)
	r.UsePattern("/admin/products/delete", vDummyHandler)

	paths := []string{"/", "/notfound", "/admin/", "/admin/foo", "/contact", "/products",
		"/products/", "/products/3/image.jpg"}
	b.StartTimer()
	for i := 0; i < b.N; i++ {

		if e := r.match(paths[i%len(paths)]); e != nil && e.pattern == "" {
			b.Error("impossible")
		}
	}
	b.StopTimer()
}

func assertRegistered(t testing.TB, router *Router, path string) {
	t.Helper()

	if _, ok := router.m[path]; !ok {
		t.Fatal("not registered the pattern")
	}
}

func assertHandler(t testing.TB, got, want Handler) {
	t.Helper()

	if got != want {
		t.Errorf("got handler %v, but want %v", got, want)
	}
}

func checkHandlerFunc(t *testing.T, router *Router, pattern, method string, handler func(ResponseWriter, *Request)) {
	t.Helper()

	e := router.m[pattern]
	got := e.mh[method].(HandlerFunc)
	assertHandlerFunc(t, got, HandlerFunc(vDummyHandlerFunc))
}

func assertHandlerFunc(t testing.TB, got HandlerFunc, want HandlerFunc) {
	t.Helper()

	if !reflect.DeepEqual(reflect.ValueOf(got), reflect.ValueOf(want)) {
		t.Errorf("got handler %#v, but want %#v", got, want)
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

func assertHandlerType(t testing.TB, want reflect.Type, got Handler) {
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

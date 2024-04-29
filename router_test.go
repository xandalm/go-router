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

func TestRouter_namespace(t *testing.T) {
	t.Run("create a namespace and return it", func(t *testing.T) {
		router := &Router{}

		nsAdmin := router.namespace("admin")

		assertRouterHasNamespace(t, router, "admin")
		if nsAdmin == nil {
			t.Error("didn't get namespace, got nil")
		}
	})
	t.Run("do not duplicate or overwritten namespace", func(t *testing.T) {
		r := &Router{}
		r.namespace("api")
		assertRouterHasNamespace(t, r, "api")
		before := r.ns["api"]

		r.namespace("api")
		assertRouterHasNamespace(t, r, "api")
		after := r.ns["api"]

		if len(r.ns) > 1 {
			t.Fatalf("namespace was duplicated, %v", r.ns)
		}

		if before != after {
			t.Errorf("namespace was overwritten, want %p but got %p", before, after)
		}

		t.Run("return the same namespace", func(t *testing.T) {
			got := r.namespace("api")
			want := after

			if got != want {
				t.Errorf("got %p but want %p", got, want)
			}
		})
	})
	t.Run("split an existent namespace if the given name is its prefix", func(t *testing.T) {
		r := &Router{}
		r.namespace("api/v1/admin")

		t.Run("router holds api and api holds v1/admin", func(t *testing.T) {
			r.namespace("api")

			if len(r.ns) != 1 {
				t.Fatal("expected that the router has 1 namespace")
			}

			assertRouterHasNamespace(t, r, "api")

			assertNamespaceHasNamespace(t, r.ns["api"], "v1/admin")
		})
		t.Run("router holds api, api holds v1 and v1 holds admin", func(t *testing.T) {
			r.namespace("api/v1")

			if len(r.ns) != 1 {
				t.Fatal("expected that the router has 1 namespace")
			}

			assertRouterHasNamespace(t, r, "api")

			apiNamespace := r.ns["api"]
			assertNamespaceHasNamespace(t, apiNamespace, "v1")

			v1Namespace := apiNamespace.ns["v1"]
			assertNamespaceHasNamespace(t, v1Namespace, "admin")
		})
	})
}

func TestRouter_register(t *testing.T) {

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

	t.Run("create namespaces indirectly", func(t *testing.T) {
		router := &Router{}

		cases := []struct {
			path   string
			method string
		}{
			{"use", MethodAll},
			{"get", MethodGet},
			{"put", MethodPut},
			{"post", MethodPost},
			{"delete", MethodDelete},
			{"admin/products", MethodGet},
			{"customers/{id}", MethodGet},
		}

		for _, c := range cases {
			pattern := "/" + c.path
			t.Run(fmt.Sprintf("registering %s method on %s", c.method, pattern), func(t *testing.T) {
				router.register(pattern, dummyHandler, c.method)

				assertRouterHasNamespace(t, router, c.path)
			})
		}
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

func TestRouter_registerFunc(t *testing.T) {

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

			router.registerFunc(c.pattern, dummyHandlerFunc, c.method)

			assertRegistered(t, router, c.pattern)

			checkHandlerFunc(t, router, c.pattern, c.method, dummyHandlerFunc)
		})
	}
}

func TestRouter_Handler(t *testing.T) {

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
			"site.com/users",
			"http://site.com:3000/users",
			"site.com/users",
			reflect.TypeOf(dummyHandler),
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

			router.Use(c.pattern, dummyHandler)

			request, _ := http.NewRequest(http.MethodGet, c.uri, nil)

			var pat string
			var h RouteHandler
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

		router.Use("/users/", handlerOne)
		router.Use("/users", handlerTwo)

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
			var h RouteHandler
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

func TestRouter_Use(t *testing.T) {

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

func TestRouter_Get(t *testing.T) {

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

func TestRouter_Post(t *testing.T) {

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

func TestRouter_Put(t *testing.T) {

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

func TestRouter_Delete(t *testing.T) {

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

func TestRouter_Namespace(t *testing.T) {
	t.Run("create a namespace and return it", func(t *testing.T) {
		router := NewRouter()

		nsAdmin := router.Namespace("admin")

		assertRouterHasNamespace(t, router, "admin")
		if nsAdmin == nil {
			t.Error("didn't get namespace, got nil")
		}
	})
}

func TestRouterNamespace_Namespace(t *testing.T) {
	t.Run("create namespace from a namespace", func(t *testing.T) {
		n := &routerNamespace{
			NewRouter(),
			nil,
			map[string]*routerNamespace{},
		}

		nn := n.Namespace("v1")

		assertNamespaceHasNamespace(t, n, "v1")

		got := n.ns["v1"]
		if got != nn {
			t.Fatalf("didn't get the namespace")
		}

		if got.r != n.r {
			t.Fatalf("got namespace with router %p, but want router %p", got.r, n.r)
		}

		if got.p != n {
			t.Fatalf("the namespace parent is not %p, got %p", n, got.p)
		}

		t.Run("return the previous created namespace", func(t *testing.T) {
			got := n.Namespace("v1")

			if got != nn {
				t.Error("didn't get the previous namespace")
			}
		})
		t.Run("if prefix already exists then create a sub-namespace", func(t *testing.T) {
			n.Namespace("v1/admin/users")

			if len(n.ns) > 1 {
				t.Fatalf("there is more than one namespaces at namespace(%p), %v", n, n.ns)
			}

			assertNamespaceHasNamespace(t, n, "v1")
			assertNamespaceHasNamespace(t, n.ns["v1"], "admin/users")
		})
		t.Run("split an existent namespace if the given name is its prefix", func(t *testing.T) {
			n.Namespace("v1/admin")

			assertNamespaceHasNamespace(t, n, "v1")
			v1 := n.ns["v1"]
			assertNamespaceHasNamespace(t, v1, "admin")
			admin := v1.ns["admin"]
			assertNamespaceHasNamespace(t, admin, "users")
		})
	})
	t.Run("namespace is reachable from the router", func(t *testing.T) {
		r := NewRouter()
		api := r.Namespace("api")

		v1 := api.Namespace("v1")

		got := r.Namespace("api/v1")

		if got != v1 {
			t.Error("unable to reach namespace from the router")
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
	r.Use("/", dummyHandler)
	r.Use("/index", dummyHandler)
	r.Use("/home", dummyHandler)
	r.Use("/about", dummyHandler)
	r.Use("/contact", dummyHandler)
	r.Use("/robots.txt", dummyHandler)
	r.Use("/products/", dummyHandler)
	r.Use("/products/{id}", dummyHandler)
	r.Use("/products/{id}/image.jpg", dummyHandler)
	r.Use("/admin", dummyHandler)
	r.Use("/admin/products/", dummyHandler)
	r.Use("/admin/products/create", dummyHandler)
	r.Use("/admin/products/update", dummyHandler)
	r.Use("/admin/products/delete", dummyHandler)

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

func assertHandler(t testing.TB, got, want RouteHandler) {
	t.Helper()

	if got != want {
		t.Errorf("got handler %v, but want %v", got, want)
	}
}

func checkHandlerFunc(t *testing.T, router *Router, pattern, method string, handler func(ResponseWriter, *Request)) {
	t.Helper()

	e := router.m[pattern]
	got := e.mh[method].(RouteHandlerFunc)
	assertHandlerFunc(t, got, RouteHandlerFunc(dummyHandlerFunc))
}

func assertHandlerFunc(t testing.TB, got RouteHandlerFunc, want RouteHandlerFunc) {
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

func assertRouterHasNamespace(t testing.TB, r *Router, n string) {
	t.Helper()

	if _, ok := r.ns[n]; !ok {
		t.Fatalf("there is no %q namespace in %v", n, r.ns)
	}
}

func assertNamespaceHasNamespace(t testing.TB, rn *routerNamespace, n string) {
	t.Helper()

	if _, ok := rn.ns[n]; !ok {
		t.Fatalf("there is no %q namespace in %v", n, rn.ns)
	}
}

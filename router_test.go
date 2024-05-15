package router

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

type dummyHandler struct{}

func (h *dummyHandler) ServeHTTP(w ResponseWriter, r *Request) {
}

var vDummyHandler = &dummyHandler{}

func TestRouter_namespace(t *testing.T) {
	t.Run("create a namespace and return it", func(t *testing.T) {
		router := &Router{}

		cases := []struct {
			namespace, check string
		}{
			{"admin", "admin"},
			{"api/v1", "api/v1"},
			{"images/{img}", "images/{}"},
			{"videos/{v}/frame/{f}", "videos/{}/frame/{}"},
			{"path/{p1}/{p2}", "path/{}/{}"},
		}

		for _, c := range cases {
			n := router.namespace(c.namespace)

			assertRouterHasNamespace(t, router, c.check)
			if n == nil {
				t.Error("didn't get namespace")
			}
		}
	})
	t.Run("panic if the given namespace starts with param", func(t *testing.T) {
		router := &Router{}

		cases := []string{
			"{param}",
			"{param}/abc",
			"{param1}/{param2}",
		}

		for _, name := range cases {
			t.Run("for namespace name "+name, func(t *testing.T) {
				defer func() {
					r := recover()
					if r == nil || r != ErrNamespaceStartsWithParam {
						t.Errorf("didn't get expected panic, got %v", r)
					}
				}()
				router.namespace(name)
			})
		}
	})
	t.Run("create a namespace in a correspondent prefix namespace", func(t *testing.T) {
		router := &Router{}

		cases := []struct {
			base        string
			namespace   string
			inRouter    string
			inNamespace string
		}{
			{"admin", "admin/users", "admin", "users"},
			{"customers/{}", "customers/{}/addresses", "customers/{}", "addresses"},
		}

		for _, c := range cases {
			n := router.namespace(c.base)
			router.namespace(c.namespace)
			assertRouterHasNamespace(t, router, c.inRouter)
			assertNamespaceHasNamespace(t, n, c.inNamespace)
		}
	})
	t.Run("if the given name is a prefix of the an existent namespace then split", func(t *testing.T) {
		r := &Router{}
		r.namespace("api/v1/admin")

		r.namespace("api")
		if len(r.ns) != 1 {
			t.Fatal("expected that the router has 1 namespace")
		}
		assertRouterHasNamespace(t, r, "api")
		assertNamespaceHasNamespace(t, r.ns["api"], "v1/admin")

		r.namespace("api/v1")
		if len(r.ns) != 1 {
			t.Fatal("expected that the router has 1 namespace")
		}
		assertRouterHasNamespace(t, r, "api")
		apiNamespace := r.ns["api"]
		assertNamespaceHasNamespace(t, apiNamespace, "v1")
		v1Namespace := apiNamespace.ns["v1"]
		assertNamespaceHasNamespace(t, v1Namespace, "admin")

		r.namespace("customers/{}")

		r.namespace("customers")
		if len(r.ns) != 2 {
			t.Fatal("expected that the router has 2 namespace")
		}
		assertRouterHasNamespace(t, r, "customers")
		assertNamespaceHasNamespace(t, r.ns["customers"], "{}")
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
}

func TestRouter_register(t *testing.T) {

	t.Run("panic on invalid pattern", func(t *testing.T) {

		cases := []string{
			"",
			"//",
			"///",
			"/path//",
			"url//",
		}

		for _, pattern := range cases {
			t.Run(fmt.Sprintf("for %q pattern", pattern), func(t *testing.T) {
				router := &Router{}

				defer func() {
					r := recover()
					if r == nil {
						t.Error("didn't panic")
					}
				}()
				router.register(pattern, vDummyHandler, MethodAll)
			})
		}
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

	t.Run("create namespaces indirectly", func(t *testing.T) {
		router := &Router{}

		cases := []struct {
			pattern   string
			method    string
			namespace string
		}{
			{"/use", MethodAll, "use"},
			{"/get", MethodGet, "get"},
			{"/put", MethodPut, "put"},
			{"/post", MethodPost, "post"},
			{"/delete", MethodDelete, "delete"},
			{"/admin/products", MethodGet, "admin/products"},
			{"/customers/{id}", MethodGet, "customers/{}"},
		}

		for _, c := range cases {
			t.Run(fmt.Sprintf("registering %s method on %s", c.method, c.pattern), func(t *testing.T) {
				router.register(c.pattern, vDummyHandler, c.method)

				assertRouterHasNamespace(t, router, c.namespace)
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
		{"/", regexp.MustCompile(`^\/?$`), MethodGet},
	}

	router := &Router{}

	for _, c := range cases {
		t.Run(fmt.Sprintf(`add %q to %s`, c.pattern, c.method), func(t *testing.T) {

			router.register(c.pattern, vDummyHandler, c.method)

			checkRegisteredEntry(t, router, c.pattern, c.re, c.method, vDummyHandler)
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
		{
			"/",
			newDummyURI(""),
			"/",
			reflect.TypeOf(vDummyHandler),
			Params{},
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("when listen to %q and request %q", c.pattern, c.uri), func(t *testing.T) {
			router := NewRouter()

			router.All(c.pattern, vDummyHandler)

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

		router.All("/users/", handlerOne)
		router.All("/users", handlerTwo)

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

func TestRouter_All(t *testing.T) {

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

			router.All(c.path, c.handler)

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

func TestRouter_Post(t *testing.T) {

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

func TestRouter_Put(t *testing.T) {

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

func TestRouter_Delete(t *testing.T) {

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
			nil,
			nil,
			nil,
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

type stubMiddleware struct {
}

func (m *stubMiddleware) Intercept(w ResponseWriter, r *Request, next NextMiddlewareCaller) {
}

var dummyMiddleware = &stubMiddleware{}

type spyMiddleware struct {
	intercepted bool
}

func (m *spyMiddleware) Intercept(w ResponseWriter, r *Request, next NextMiddlewareCaller) {
	m.intercepted = true
	next()
}

func TestRouter_Use(t *testing.T) {

	t.Run("create middleware into router", func(t *testing.T) {
		router := NewRouter()
		router.Use(dummyMiddleware)

		if len(router.mws) != 1 {
			t.Fatal("didn't create middleware appropriately")
		}

		got := router.mws[0]
		want := dummyMiddleware

		if got != want {
			t.Errorf("got middleware %v, but want %v", got, want)
		}
	})

	t.Run("router middleware can intercept requests", func(t *testing.T) {

		cases := [][]*spyMiddleware{
			{&spyMiddleware{}},
			{&spyMiddleware{}, &spyMiddleware{}},
			{&spyMiddleware{}, &spyMiddleware{}, &spyMiddleware{}},
		}

		for _, mws := range cases {
			router := NewRouter()
			t.Run(fmt.Sprintf("request intercepted by %d middlewares", len(mws)), func(t *testing.T) {
				for _, mw := range mws {
					router.Use(mw)
				}

				req, _ := http.NewRequest(http.MethodGet, newDummyURI(""), nil)

				router.ServeHTTP(httptest.NewRecorder(), req)

				for i, mw := range mws {
					if !mw.intercepted {
						t.Errorf("middleware %d didn't intercept request, got %t", i+1, mw.intercepted)
					}
				}
			})
		}
	})

	t.Run("able to add many middleware in the same call", func(t *testing.T) {
		r := NewRouter()
		r.Use(dummyMiddleware, dummyMiddleware, dummyMiddleware)

		if len(r.mws) != 3 {
			t.Errorf("expected to get 3 middlewares, but got %d", len(r.mws))
		}
	})

	t.Run("able to add middleware to a specific namespace accordingly to pattern/path", func(t *testing.T) {
		r := NewRouter()

		r.Use("/path", dummyMiddleware)

		if len(r.mws) > 0 {
			t.Fatal("expected no middleware in the router")
		}

		n := r.Namespace("path")

		if len(n.mws) != 1 {
			t.Fatalf("expected to get 1 middleware in the namespace, but get %d", len(n.mws))
		}

		got := n.mws[0].(*stubMiddleware)

		if got != dummyMiddleware {
			t.Errorf("got middleware %v, but want %v", got, dummyMiddleware)
		}
	})
}

func TestRouterNamespace_Use(t *testing.T) {
	t.Run("create middleware into namespace", func(t *testing.T) {
		r := NewRouter()
		n := r.Namespace("api")

		n.Use(dummyMiddleware)

		if len(n.mws) != 1 {
			t.Fatal("didn't create middleware appropriately")
		}

		got := n.mws[0]
		want := dummyMiddleware

		if got != want {
			t.Errorf("got middleware %v, but want %v", got, want)
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
	r.All("/", vDummyHandler)
	r.All("/index", vDummyHandler)
	r.All("/home", vDummyHandler)
	r.All("/about", vDummyHandler)
	r.All("/contact", vDummyHandler)
	r.All("/robots.txt", vDummyHandler)
	r.All("/products/", vDummyHandler)
	r.All("/products/{id}", vDummyHandler)
	r.All("/products/{id}/image.jpg", vDummyHandler)
	r.All("/admin", vDummyHandler)
	r.All("/admin/products/", vDummyHandler)
	r.All("/admin/products/create", vDummyHandler)
	r.All("/admin/products/update", vDummyHandler)
	r.All("/admin/products/delete", vDummyHandler)

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

func closestNamespace(router *Router, path string) (n *routerNamespace, p string) {
	ns := router.ns

	path = parseNamespace(path)
	r := regexp.MustCompile(`\/[^\/]+$`)
	var search string
	for path != "" {
		search = path
		for search != "" {
			if f, ok := ns[search]; ok {
				path = strings.TrimPrefix(strings.TrimPrefix(path, search), "/")
				ns = f.ns
				n = f
				break
			}
			if f, ok := ns["{}"]; ok {
				path = strings.TrimPrefix(strings.TrimPrefix(path, search), "/")
				ns = f.ns
				n = f
				break
			}
			search = r.ReplaceAllString(search, "")
		}
	}

	return
}

func findRouterEntry(router *Router, path string) (unslashed *routerEntry, slashed *routerEntry) {
	n, p := closestNamespace(router, path)

	if n == nil || p != "" {
		return nil, nil
	}

	return n.eu, n.es
}

func checkRegisteredEntry(t *testing.T, router *Router, pattern string, re *regexp.Regexp, method string, handler Handler) {
	t.Helper()

	eu, es := findRouterEntry(router, pattern)

	var e *routerEntry
	switch {
	case eu == nil && es == nil:
		e = router.e
	case eu != nil:
		e = eu
	case es != nil:
		e = es
	}

	if e == nil {
		t.Fatal("didn't registered entry")
	}

	assertHandler(t, e.mh[method], handler)

	if !reflect.DeepEqual(re, e.re) {
		t.Errorf("got regexp %q, but want %q", e.re, re)
	}
}

func assertHandler(t testing.TB, got, want Handler) {
	t.Helper()

	if got != want {
		t.Fatalf("got handler %v, but want %v", got, want)
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

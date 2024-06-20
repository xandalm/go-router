package router

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var dummyHandler = &stubHandler{}

func TestRouter_namespace(t *testing.T) {
	t.Run("create a namespace and return it", func(t *testing.T) {

		cases := []struct {
			namespace, check string
		}{
			{"admin", "admin"},
			{"api/v1", "api/v1"},
			{"images/{}", "images/{}"},
			{"videos/{}/frame/{}", "videos/{}/frame/{}"},
			{"path/{}/{}", "path/{}/{}"},
			{"{}", "{}"},
			{"{}/{}", "{}/{}"},
		}

		for _, c := range cases {
			router := &Router{}
			n := router.namespace(c.namespace)

			assertRouterHasNamespace(t, router, c.check)
			if n == nil {
				t.Error("didn't get namespace")
			}
		}
	})
	t.Run("create a namespace in a corresponding prefix namespace", func(t *testing.T) {
		router := &Router{}

		cases := []struct {
			base        string
			namespace   string
			inRouter    string
			inNamespace string
		}{
			{"admin", "admin/users", "admin", "users"},
			{"customers/{}", "customers/{}/addresses", "customers/{}", "addresses"},
			{"{}", "{}/data", "{}", "data"},
		}

		for _, c := range cases {
			n := router.namespace(c.base)
			router.namespace(c.namespace)
			assertRouterHasNamespace(t, router, c.inRouter)
			assertRouterNamespaceHasNamespace(t, n, c.inNamespace)
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
		assertRouterNamespaceHasNamespace(t, r.ns["api"], "v1/admin")

		r.namespace("api/v1")
		if len(r.ns) != 1 {
			t.Fatal("expected that the router has 1 namespace")
		}
		assertRouterHasNamespace(t, r, "api")
		apiNamespace := r.ns["api"]
		assertRouterNamespaceHasNamespace(t, apiNamespace, "v1")
		v1Namespace := apiNamespace.ns["v1"]
		assertRouterNamespaceHasNamespace(t, v1Namespace, "admin")

		r.namespace("customers/{}")

		r.namespace("customers")
		if len(r.ns) != 2 {
			t.Fatal("expected that the router has 2 namespace")
		}
		assertRouterHasNamespace(t, r, "customers")
		assertRouterNamespaceHasNamespace(t, r.ns["customers"], "{}")
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
			"/users/{}",
		}

		for _, pattern := range cases {
			t.Run(fmt.Sprintf("for %q pattern", pattern), func(t *testing.T) {
				router := &Router{}

				defer func() {
					r := recover()
					if r == nil {
						t.Fatal("didn't panic")
					}
					if r != PanicMsgInvalidPattern {
						t.Errorf("panics %v, but want %v", r, PanicMsgInvalidPattern)
					}
				}()
				router.register(pattern, dummyHandler, MethodAll)
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
			if r != PanicMsgEmptyHandler {
				t.Errorf("panics %v, but want %v", r, PanicMsgEmptyHandler)
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
			if r != PanicMsgEndpointDuplication {
				t.Errorf("panics %v, but want %v", r, PanicMsgEndpointDuplication)
			}
		}()

		router.register("/path", dummyHandler, MethodAll)
		router.register("/path", dummyHandler, MethodAll)
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
				router.register(c.pattern, dummyHandler, c.method)

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

			router.register(c.pattern, dummyHandler, c.method)

			checkRegisteredEntry(t, router, c.pattern, c.re, c.method, dummyHandler)
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
			if r != PanicMsgEmptyHandler {
				t.Errorf("panics %v, but want %v", r, PanicMsgEmptyHandler)
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
		{
			"/",
			newDummyURI(""),
			"/",
			reflect.TypeOf(dummyHandler),
			Params{},
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("when listen to %q and request %q", c.pattern, c.uri), func(t *testing.T) {
			router := NewRouter()

			router.register(c.pattern, dummyHandler, http.MethodGet)

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

		handlerOne := &mockHandler{
			OnHandleFunc: func(w ResponseWriter, r *Request) {
			},
		}
		handlerTwo := &mockHandler{
			OnHandleFunc: func(w ResponseWriter, r *Request) {
			},
		}

		router.register("/users/", handlerOne, http.MethodGet)
		router.register("/users", handlerTwo, http.MethodGet)

		cases := []struct {
			path    string
			pattern string
			handler *mockHandler
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

	t.Run("returns not found handler instead of redirect handler if the method is not registered", func(t *testing.T) {

		testFn := func(caller func(r *Router, p string, h Handler), pattern string, path string, methods []string) {
			router := NewRouter()
			caller(router, pattern, dummyHandler)
			for _, m := range methods {
				request, _ := http.NewRequest(m, newDummyURI(path), nil)

				h, pat, params := router.Handler(request)

				assertHandler(t, h, NotFoundHandler)

				if pat != "" {
					t.Errorf("got pattern %q, but want empty string", pat)
				}

				assertParams(t, params, nil)
			}
		}

		toTestFn := []struct {
			fn      func(r *Router, p string, h Handler)
			methods []string
		}{
			{
				func(r *Router, p string, h Handler) {
					r.Get(p, h)
				},
				[]string{MethodPost, MethodPut, MethodDelete},
			},
			{
				func(r *Router, p string, h Handler) {
					r.Post(p, h)
				},
				[]string{MethodGet, MethodPut, MethodDelete},
			},
			{
				func(r *Router, p string, h Handler) {
					r.Put(p, h)
				},
				[]string{MethodPost, MethodGet, MethodDelete},
			},
			{
				func(r *Router, p string, h Handler) {
					r.Delete(p, h)
				},
				[]string{MethodPost, MethodPut, MethodGet},
			},
		}

		cases := []struct {
			pattern, path string
		}{
			{
				"/path",
				"/path/",
			},
			{
				"/path/",
				"/path",
			},
		}

		for _, c := range cases {
			for _, tt := range toTestFn {
				testFn(
					tt.fn,
					c.pattern,
					c.path,
					tt.methods,
				)
			}
		}
	})
}

type testResquestUsingHandler struct {
	name            string // test case description
	uri             string
	httpMethod      string
	expectedHandler Handler
	expectedPattern string
	expectedParams  Params
}

type testRequestUsingServeHTTP struct {
	name           string // test case description
	uri            string
	method         string
	expectedStatus int
	expectedBody   string
}

func TestRouter_All(t *testing.T) {

	type testCase struct {
		path     string
		uriTests []testResquestUsingHandler
	}

	cases := []testCase{
		{
			"/users",
			[]testResquestUsingHandler{
				{
					uri:             newDummyURI("/users"),
					httpMethod:      http.MethodGet,
					expectedHandler: dummyHandler,
					expectedParams:  Params{},
				},
				{
					uri:             newDummyURI("/users"),
					httpMethod:      http.MethodPost,
					expectedHandler: dummyHandler,
					expectedParams:  Params{},
				},
				{
					uri:             newDummyURI("/users"),
					httpMethod:      http.MethodPut,
					expectedHandler: dummyHandler,
					expectedParams:  Params{},
				},
				{
					uri:             newDummyURI("/users"),
					httpMethod:      http.MethodDelete,
					expectedHandler: dummyHandler,
					expectedParams:  Params{},
				},
			},
		},
		{
			"/users/{id}",
			[]testResquestUsingHandler{
				{
					uri:             newDummyURI("/users/13"),
					httpMethod:      http.MethodGet,
					expectedHandler: dummyHandler,
					expectedParams:  Params{"id": "13"},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("add %s pattern", c.path), func(t *testing.T) {
			router := &Router{}

			router.All(c.path, dummyHandler)

			for _, tt := range c.uriTests {
				t.Run(fmt.Sprintf("%s to %s returns expected handler and params", tt.httpMethod, tt.uri), func(t *testing.T) {
					request, _ := http.NewRequest(tt.httpMethod, tt.uri, nil)

					h, _, params := router.Handler(request)

					assertHandler(t, h, dummyHandler)
					assertParams(t, params, tt.expectedParams)
				})
			}
		})
	}
}

func TestRouter_Get(t *testing.T) {

	t.Run(`router with only "/products" on GET`, func(t *testing.T) {
		router := NewRouter()

		router.Get("/products", dummyHandler)

		cases := []testResquestUsingHandler{
			{
				name:            "returns handler and empty params",
				uri:             newDummyURI("/products"),
				httpMethod:      http.MethodGet,
				expectedHandler: dummyHandler,
				expectedParams:  Params{},
			},
			{
				name:            "returns nil handler and nil params",
				uri:             newDummyURI("/products"),
				httpMethod:      http.MethodPost,
				expectedHandler: NotFoundHandler,
				expectedParams:  nil,
			},
			{
				name:            "returns nil handler and nil params",
				uri:             newDummyURI("/products"),
				httpMethod:      http.MethodPut,
				expectedHandler: NotFoundHandler,
				expectedParams:  nil,
			},
			{
				name:            "returns nil handler and nil params",
				uri:             newDummyURI("/products"),
				httpMethod:      http.MethodDelete,
				expectedHandler: NotFoundHandler,
				expectedParams:  nil,
			},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				request, _ := http.NewRequest(c.httpMethod, c.uri, nil)

				h, _, params := router.Handler(request)

				assertHandler(t, h, c.expectedHandler)
				assertParams(t, params, c.expectedParams)
			})
		}
	})
}

func TestRouter_Post(t *testing.T) {

	t.Run(`router with only "/products" on POST`, func(t *testing.T) {
		router := NewRouter()

		router.Post("/products", dummyHandler)

		cases := []testResquestUsingHandler{
			{
				name:            "returns handler and empty params",
				uri:             newDummyURI("/products"),
				httpMethod:      http.MethodPost,
				expectedHandler: dummyHandler,
				expectedParams:  Params{},
			},
			{
				name:            "returns nil handler and nil params",
				uri:             newDummyURI("/products"),
				httpMethod:      http.MethodGet,
				expectedHandler: NotFoundHandler,
				expectedParams:  nil,
			},
			{
				name:            "returns nil handler and nil params",
				uri:             newDummyURI("/products"),
				httpMethod:      http.MethodPut,
				expectedHandler: NotFoundHandler,
				expectedParams:  nil,
			},
			{
				name:            "returns nil handler and nil params",
				uri:             newDummyURI("/products"),
				httpMethod:      http.MethodDelete,
				expectedHandler: NotFoundHandler,
				expectedParams:  nil,
			},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				request, _ := http.NewRequest(c.httpMethod, c.uri, nil)

				h, _, params := router.Handler(request)

				assertHandler(t, h, c.expectedHandler)
				assertParams(t, params, c.expectedParams)
			})
		}
	})
}

func TestRouter_Put(t *testing.T) {

	t.Run(`router with only "/products" on PUT`, func(t *testing.T) {
		router := NewRouter()

		router.Put("/products", dummyHandler)

		cases := []testResquestUsingHandler{
			{
				name:            "returns handler and empty params",
				uri:             newDummyURI("/products"),
				httpMethod:      http.MethodPut,
				expectedHandler: dummyHandler,
				expectedParams:  Params{},
			},
			{
				name:            "returns nil handler and nil params",
				uri:             newDummyURI("/products"),
				httpMethod:      http.MethodGet,
				expectedHandler: NotFoundHandler,
				expectedParams:  nil,
			},
			{
				name:            "returns nil handler and nil params",
				uri:             newDummyURI("/products"),
				httpMethod:      http.MethodPost,
				expectedHandler: NotFoundHandler,
				expectedParams:  nil,
			},
			{
				name:            "returns nil handler and nil params",
				uri:             newDummyURI("/products"),
				httpMethod:      http.MethodDelete,
				expectedHandler: NotFoundHandler,
				expectedParams:  nil,
			},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				request, _ := http.NewRequest(c.httpMethod, c.uri, nil)

				h, _, params := router.Handler(request)

				assertHandler(t, h, c.expectedHandler)
				assertParams(t, params, c.expectedParams)
			})
		}
	})
}

func TestRouter_Delete(t *testing.T) {

	t.Run(`router with only "/products" on DELETE`, func(t *testing.T) {
		router := NewRouter()

		router.Delete("/products", dummyHandler)

		cases := []testResquestUsingHandler{
			{
				name:            "returns handler and empty params",
				uri:             newDummyURI("/products"),
				httpMethod:      http.MethodDelete,
				expectedHandler: dummyHandler,
				expectedParams:  Params{},
			},
			{
				name:            "returns nil handler and nil params",
				uri:             newDummyURI("/products"),
				httpMethod:      http.MethodGet,
				expectedHandler: NotFoundHandler,
				expectedParams:  nil,
			},
			{
				name:            "returns nil handler and nil params",
				uri:             newDummyURI("/products"),
				httpMethod:      http.MethodPut,
				expectedHandler: NotFoundHandler,
				expectedParams:  nil,
			},
			{
				name:            "returns nil handler and nil params",
				uri:             newDummyURI("/products"),
				httpMethod:      http.MethodPost,
				expectedHandler: NotFoundHandler,
				expectedParams:  nil,
			},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				request, _ := http.NewRequest(c.httpMethod, c.uri, nil)

				h, _, params := router.Handler(request)

				assertHandler(t, h, c.expectedHandler)
				assertParams(t, params, c.expectedParams)
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
	t.Run("panic for invalid namespace", func(t *testing.T) {
		type testCase struct {
			testName string
			value    string
		}
		cases := []testCase{
			{"when starts with bar", "/media"},
			{"when contains a unnamed param like", "users/{}"},
		}

		for _, c := range cases {
			t.Run(c.testName, func(t *testing.T) {
				defer func() {
					r := recover()
					if r == nil {
						t.Fatal("didn't panic")
					}
					if r != PanicMsgInvalidNamespace {
						t.Errorf("panics %v, but want %v", r, PanicMsgInvalidNamespace)
					}
				}()
				r := NewRouter()
				r.Namespace(c.value)
			})
		}
	})
}

func TestNamespace_Namespace(t *testing.T) {
	t.Run("create namespace from a namespace", func(t *testing.T) {
		n := &namespace{
			n: &routerNamespace{
				"v1",
				NewRouter(),
				nil,
				map[string]*routerNamespace{},
				nil,
				nil,
				nil,
			},
		}

		nn := n.Namespace("v1")

		assertNamespaceHasNamespace(t, n, "v1")

		got := n.n.ns["v1"]
		if got != nn.n {
			t.Fatalf("didn't get the namespace")
		}

		if got.r != n.n.r {
			t.Fatalf("got namespace with router %p, but want router %p", got.r, n.n.r)
		}

		if got.p != n.n {
			t.Fatalf("the namespace parent is not %p, got %p", n, got.p)
		}

		t.Run("return the previous created namespace", func(t *testing.T) {
			got := n.Namespace("v1")

			if got.n != nn.n {
				t.Error("didn't get the previous namespace")
			}
		})
		t.Run("if prefix already exists then create a sub-namespace", func(t *testing.T) {
			n.Namespace("v1/admin/users")

			if len(n.n.ns) > 1 {
				t.Fatalf("there is more than one namespaces at namespace(%p), %v", n, n.n.ns)
			}

			assertNamespaceHasNamespace(t, n, "v1")
			assertNamespaceHasNamespace(t, n.Namespace("v1"), "admin/users")
		})
		t.Run("split an existent namespace if the given name is its prefix", func(t *testing.T) {
			n.Namespace("v1/admin")

			assertNamespaceHasNamespace(t, n, "v1")
			v1 := n.Namespace("v1")
			assertNamespaceHasNamespace(t, v1, "admin")
			admin := v1.Namespace("admin")
			assertNamespaceHasNamespace(t, admin, "users")
		})
	})
	t.Run("namespace is reachable from the router", func(t *testing.T) {
		r := NewRouter()
		api := r.Namespace("api")

		v1 := api.Namespace("v1")

		got := r.Namespace("api/v1")

		if got.n != v1.n {
			t.Error("unable to reach namespace from the router")
		}
	})
	t.Run("panic for invalid namespace", func(t *testing.T) {
		type testCase struct {
			testName string
			value    string
		}
		cases := []testCase{
			{"when starts with bar", "/media"},
			{"when contains a unnamed param like", "{}"},
		}
		api := &namespace{
			n: &routerNamespace{
				"api",
				NewRouter(),
				nil,
				map[string]*routerNamespace{},
				nil,
				nil,
				nil,
			},
		}
		for _, c := range cases {
			t.Run(c.testName, func(t *testing.T) {
				defer func() {
					r := recover()
					if r == nil {
						t.Fatal("didn't panic")
					}
					if r != PanicMsgInvalidNamespace {
						t.Errorf("panics %v, but want %v", r, PanicMsgInvalidNamespace)
					}
				}()
				api.Namespace(c.value)
			})
		}
	})
}

func TestNamespace_register(t *testing.T) {

	t.Run("panic on invalid pattern", func(t *testing.T) {

		cases := []string{
			"//",
			"///",
			"/path//",
			"url//",
			"/users/{}",
		}

		for _, pattern := range cases {
			t.Run(fmt.Sprintf("for %q pattern", pattern), func(t *testing.T) {
				router := &Router{}
				namespace := router.Namespace("api")

				defer func() {
					r := recover()
					if r == nil {
						t.Fatal("didn't panic")
					}
					if r != PanicMsgInvalidPattern {
						t.Errorf("panics %v, but want %v", r, PanicMsgInvalidPattern)
					}
				}()
				namespace.register(pattern, dummyHandler, MethodAll)
			})
		}
	})

	t.Run("panic on nil handler", func(t *testing.T) {
		router := &Router{}
		namespace := router.Namespace("api")

		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
			if r != PanicMsgEmptyHandler {
				t.Errorf("panics %v, but want %v", r, PanicMsgEmptyHandler)
			}
		}()

		namespace.register("/path", nil, MethodAll)
	})

	t.Run("panic on re-register same pattern and method", func(t *testing.T) {
		router := &Router{}
		namespace := router.Namespace("api")

		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
			if r != PanicMsgEndpointDuplication {
				t.Errorf("panics %v, but want %v", r, PanicMsgEndpointDuplication)
			}
		}()

		namespace.register("/path", dummyHandler, MethodAll)
		namespace.register("/path", dummyHandler, MethodAll)
	})

	t.Run("create namespaces indirectly", func(t *testing.T) {
		router := &Router{}
		namespace := router.Namespace("api")

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
			t.Run(fmt.Sprintf("registering %s method on %s with api namespace", c.method, c.pattern), func(t *testing.T) {
				namespace.register(c.pattern, dummyHandler, c.method)

				assertNamespaceHasNamespace(t, namespace, c.namespace)
			})
		}
	})

	userRE := regexp.MustCompile(`^\/api\/users$`)

	cases := []struct {
		pattern string
		re      *regexp.Regexp
		method  string
	}{
		{"/users", userRE, MethodAll},
		{"/v1/users", regexp.MustCompile(`^\/api\/v1\/users$`), MethodAll},
		{"/users", userRE, MethodGet},
		{"/users", userRE, MethodPost},
		{"/users", userRE, MethodPut},
		{"/users", userRE, MethodDelete},
		{"/users/{id}", regexp.MustCompile(`^\/api\/users\/(?P<id>[^\/]+)$`), MethodGet},
		{"/", regexp.MustCompile(`^\/api\/$`), MethodGet},
	}

	router := &Router{}
	namespace := router.Namespace("api")

	for _, c := range cases {
		t.Run(fmt.Sprintf(`add %q to %s`, c.pattern, c.method), func(t *testing.T) {

			namespace.register(c.pattern, dummyHandler, c.method)

			checkRegisteredEntry(t, router, "/api"+c.pattern, c.re, c.method, dummyHandler)
		})
	}
}

func testCommonCasesOnNamespace__All_Get_Post_Put_or_Delete(t *testing.T, caller func(*namespace, any, ...Handler), method string) {
	type testCase struct {
		name      string
		namespace string
		path      string
		uriTests  []testResquestUsingHandler
	}

	cases := []testCase{
		{
			"add handler to \"/\" in a simple namespace",
			"users",
			"/",
			[]testResquestUsingHandler{
				{
					name:            "returns associated handler and empty params",
					uri:             newDummyURI("/users/"),
					expectedHandler: dummyHandler,
					expectedParams:  Params{},
				},
				{
					name:            "returns redirect handler and nil to params",
					uri:             newDummyURI("/users"),
					expectedHandler: RedirectHandler("/users/", http.StatusMovedPermanently),
					expectedParams:  nil,
				},
			},
		},
		{
			"add handler in a nest layer of the namespace with a param suffix",
			"users/{id}",
			"/gifs",
			[]testResquestUsingHandler{
				{
					name:            "returns associated handler and [id=1] into params",
					uri:             newDummyURI("/users/1/gifs"),
					expectedHandler: dummyHandler,
					expectedParams:  Params{"id": "1"},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			router := &Router{}
			namespace := router.Namespace(c.namespace)
			caller(namespace, c.path, dummyHandler)

			for _, cc := range c.uriTests {
				request, _ := http.NewRequest(method, cc.uri, nil)

				h, _, params := router.Handler(request)

				assertHandler(t, h, cc.expectedHandler)
				assertParams(t, params, cc.expectedParams)
			}
		})
	}

	t.Run(`able to add handler avoiding bar (to "[NAMESPACE_PATH]" instead of "[NAMESPACE_PATH]/")`, func(t *testing.T) {
		router := &Router{}
		namespace := router.Namespace("users")
		caller(namespace, dummyHandler)

		request, _ := http.NewRequest(method, newDummyURI("/users"), nil)

		h, _, params := router.Handler(request)

		assertHandler(t, h, dummyHandler)
		assertParams(t, params, Params{})
	})

	t.Run("panic on empty pattern", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
			if r != PanicMsgInvalidPattern {
				t.Errorf("panics %v, but want %v", r, PanicMsgInvalidPattern)
			}
		}()
		router := &Router{}
		namespace := router.Namespace("users")
		caller(namespace, "", dummyHandler)
	})

	t.Run("panic when give no one handler", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
			if r != PanicMsgMissingHandler {
				t.Errorf("panics %v, but want %v", r, PanicMsgMissingHandler)
			}
		}()
		router := &Router{}
		namespace := router.Namespace("users")
		caller(namespace, "/actives")
	})
}

func checkTestResquestUsingHandler(t *testing.T, caller func(*namespace, string, Handler), n, p string, cases []testResquestUsingHandler) {

	router := NewRouter()
	namespace := router.Namespace(n)
	caller(namespace, p, dummyHandler)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			request, _ := http.NewRequest(c.httpMethod, c.uri, nil)

			h, _, params := router.Handler(request)

			assertHandler(t, h, c.expectedHandler)
			assertParams(t, params, c.expectedParams)
		})
	}
}

func TestNamespace_All(t *testing.T) {
	testCommonCasesOnNamespace__All_Get_Post_Put_or_Delete(t, func(n *namespace, a any, h ...Handler) {
		n.All(a, h...)
	}, MethodGet)
}

func TestNamespace_Get(t *testing.T) {
	testCommonCasesOnNamespace__All_Get_Post_Put_or_Delete(t, func(n *namespace, a any, h ...Handler) {
		n.Get(a, h...)
	}, MethodGet)

	cases := []testResquestUsingHandler{
		{
			name:            "returns handler and empty params",
			uri:             newDummyURI("/media/images"),
			httpMethod:      http.MethodGet,
			expectedHandler: dummyHandler,
			expectedParams:  Params{},
		},
		{
			name:            "returns nil handler and nil params",
			uri:             newDummyURI("/media/images"),
			httpMethod:      http.MethodPost,
			expectedHandler: NotFoundHandler,
			expectedParams:  nil,
		},
		{
			name:            "returns nil handler and nil params",
			uri:             newDummyURI("/media/images"),
			httpMethod:      http.MethodPut,
			expectedHandler: NotFoundHandler,
			expectedParams:  nil,
		},
		{
			name:            "returns nil handler and nil params",
			uri:             newDummyURI("/media/images"),
			httpMethod:      http.MethodDelete,
			expectedHandler: NotFoundHandler,
			expectedParams:  nil,
		},
		{
			name:            "returns redirect handler and nil params",
			uri:             newDummyURI("/media/images/"),
			httpMethod:      http.MethodGet,
			expectedHandler: RedirectHandler("/media/images", http.StatusMovedPermanently),
			expectedParams:  nil,
		},
	}

	checkTestResquestUsingHandler(t, func(n *namespace, p string, h Handler) { n.Get(p, h) }, "media", "/images", cases)
}

func TestNamespace_Post(t *testing.T) {
	testCommonCasesOnNamespace__All_Get_Post_Put_or_Delete(t, func(n *namespace, a any, h ...Handler) {
		n.Post(a, h...)
	}, MethodPost)

	cases := []testResquestUsingHandler{
		{
			name:            "returns handler and empty params",
			uri:             newDummyURI("/media/images"),
			httpMethod:      http.MethodPost,
			expectedHandler: dummyHandler,
			expectedParams:  Params{},
		},
		{
			name:            "returns nil handler and nil params",
			uri:             newDummyURI("/media/images"),
			httpMethod:      http.MethodGet,
			expectedHandler: NotFoundHandler,
			expectedParams:  nil,
		},
		{
			name:            "returns nil handler and nil params",
			uri:             newDummyURI("/media/images"),
			httpMethod:      http.MethodPut,
			expectedHandler: NotFoundHandler,
			expectedParams:  nil,
		},
		{
			name:            "returns nil handler and nil params",
			uri:             newDummyURI("/media/images"),
			httpMethod:      http.MethodDelete,
			expectedHandler: NotFoundHandler,
			expectedParams:  nil,
		},
		{
			name:            "returns redirect handler and nil params",
			uri:             newDummyURI("/media/images/"),
			httpMethod:      http.MethodPost,
			expectedHandler: RedirectHandler("/media/images", http.StatusMovedPermanently),
			expectedParams:  nil,
		},
	}

	checkTestResquestUsingHandler(t, func(n *namespace, p string, h Handler) { n.Post(p, h) }, "media", "/images", cases)
}

func TestNamespace_Put(t *testing.T) {
	testCommonCasesOnNamespace__All_Get_Post_Put_or_Delete(t, func(n *namespace, a any, h ...Handler) {
		n.Put(a, h...)
	}, MethodPut)

	cases := []testResquestUsingHandler{
		{
			name:            "returns handler and empty params",
			uri:             newDummyURI("/media/images"),
			httpMethod:      http.MethodPut,
			expectedHandler: dummyHandler,
			expectedParams:  Params{},
		},
		{
			name:            "returns nil handler and nil params",
			uri:             newDummyURI("/media/images"),
			httpMethod:      http.MethodPost,
			expectedHandler: NotFoundHandler,
			expectedParams:  nil,
		},
		{
			name:            "returns nil handler and nil params",
			uri:             newDummyURI("/media/images"),
			httpMethod:      http.MethodGet,
			expectedHandler: NotFoundHandler,
			expectedParams:  nil,
		},
		{
			name:            "returns nil handler and nil params",
			uri:             newDummyURI("/media/images"),
			httpMethod:      http.MethodDelete,
			expectedHandler: NotFoundHandler,
			expectedParams:  nil,
		},
		{
			name:            "returns redirect handler and nil params",
			uri:             newDummyURI("/media/images/"),
			httpMethod:      http.MethodPut,
			expectedHandler: RedirectHandler("/media/images", http.StatusMovedPermanently),
			expectedParams:  nil,
		},
	}

	checkTestResquestUsingHandler(t, func(n *namespace, p string, h Handler) { n.Put(p, h) }, "media", "/images", cases)
}

func TestNamespace_Delete(t *testing.T) {
	testCommonCasesOnNamespace__All_Get_Post_Put_or_Delete(t, func(n *namespace, a any, h ...Handler) {
		n.Delete(a, h...)
	}, MethodDelete)

	cases := []testResquestUsingHandler{
		{
			name:            "returns handler and empty params",
			uri:             newDummyURI("/media/images"),
			httpMethod:      http.MethodDelete,
			expectedHandler: dummyHandler,
			expectedParams:  Params{},
		},
		{
			name:            "returns nil handler and nil params",
			uri:             newDummyURI("/media/images"),
			httpMethod:      http.MethodPost,
			expectedHandler: NotFoundHandler,
			expectedParams:  nil,
		},
		{
			name:            "returns nil handler and nil params",
			uri:             newDummyURI("/media/images"),
			httpMethod:      http.MethodGet,
			expectedHandler: NotFoundHandler,
			expectedParams:  nil,
		},
		{
			name:            "returns nil handler and nil params",
			uri:             newDummyURI("/media/images"),
			httpMethod:      http.MethodGet,
			expectedHandler: NotFoundHandler,
			expectedParams:  nil,
		},
		{
			name:            "returns redirect handler and nil params",
			uri:             newDummyURI("/media/images/"),
			httpMethod:      http.MethodDelete,
			expectedHandler: RedirectHandler("/media/images", http.StatusMovedPermanently),
			expectedParams:  nil,
		},
	}

	checkTestResquestUsingHandler(t, func(n *namespace, p string, h Handler) { n.Delete(p, h) }, "media", "/images", cases)
}

var dummyMiddleware = &stubMiddleware{}
var errFoo = errors.New("foo")

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

	t.Run("the middleware can interrupt a request", func(t *testing.T) {
		router := NewRouter()

		justReachTheHandler := false

		router.Use(&mockMiddleware{
			InterceptFunc: func(w ResponseWriter, r *Request, next NextMiddlewareCaller) {
				next(errFoo)
			},
		})
		router.All("/", &mockHandler{
			OnHandleFunc: func(w ResponseWriter, r *Request) {
				justReachTheHandler = true
			},
		})

		req, _ := http.NewRequest(http.MethodGet, newDummyURI(""), nil)

		router.ServeHTTP(httptest.NewRecorder(), req)

		if justReachTheHandler {
			t.Error("didn't interrupted the request")
		}
	})

	t.Run("able to add many middleware in the same call", func(t *testing.T) {
		r := NewRouter()
		r.Use(dummyMiddleware, dummyMiddleware, dummyMiddleware)

		if len(r.mws) != 3 {
			t.Errorf("expected to get 3 middlewares, but got %d", len(r.mws))
		}
	})

	t.Run("able to add middleware to a specific path", func(t *testing.T) {
		r := NewRouter()

		r.Use("/path", dummyMiddleware)

		if len(r.mws) > 0 {
			t.Fatal("expected no middleware in the router")
		}

		n := r.Namespace("path").n

		if len(n.mws) != 1 {
			t.Fatalf("expected to get 1 middleware, but get %d", len(n.mws))
		}

		got := n.mws[0].(*stubMiddleware)

		if got != dummyMiddleware {
			t.Errorf("got middleware %v, but want %v", got, dummyMiddleware)
		}
	})

	t.Run("add a middleware error handler", func(t *testing.T) {
		r := NewRouter()

		m := &spyMiddlewareErrorHandler{}
		r.Use(m)

		got := r.meh

		if got != m {
			t.Errorf("got middleware error handler %v, but want %v", got, m)
		}

		t.Run("can handle error caused by middleware", func(t *testing.T) {

			r.Use(&mockMiddleware{
				InterceptFunc: func(w ResponseWriter, r *Request, next NextMiddlewareCaller) {
					next(errFoo)
				},
			})

			req, _ := http.NewRequest(http.MethodGet, newDummyURI(""), nil)

			r.ServeHTTP(httptest.NewRecorder(), req)

			if m.calls != 1 {
				t.Errorf("didn't handle with middleware error properly")
			}
		})
	})

	t.Run("UseFunc able to add func as middleware", func(t *testing.T) {
		r := NewRouter()

		dummyMiddlewareFunc := func(w ResponseWriter, r *Request, next NextMiddlewareCaller) {}
		dummyMiddlewareErrorHandlerFunc := func(w ResponseWriter, r *Request, err error) {}

		t.Run("adding to router", func(t *testing.T) {

			r.UseFunc(dummyMiddlewareFunc)

			got := reflect.ValueOf(r.mws[0]).Pointer()
			want := reflect.ValueOf(dummyMiddlewareFunc).Pointer()

			if got != want {
				t.Errorf("got %#v, but want %#v", got, want)
			}

		})
		t.Run("adding to router path", func(t *testing.T) {

			r.UseFunc("/api", dummyMiddlewareFunc)

			got := reflect.ValueOf(r.mws[0]).Pointer()
			want := reflect.ValueOf(dummyMiddlewareFunc).Pointer()

			if got != want {
				t.Errorf("got %#v, but want %#v", got, want)
			}

		})
		t.Run("adding middleware error handler", func(t *testing.T) {

			r.UseFunc(dummyMiddlewareErrorHandlerFunc)

			got := reflect.ValueOf(r.meh).Pointer()
			want := reflect.ValueOf(dummyMiddlewareErrorHandlerFunc).Pointer()

			if got != want {
				t.Errorf("got %#v, but want %#v", got, want)
			}
		})
	})
}

func TestNamespace_Use(t *testing.T) {
	t.Run("create middleware into namespace", func(t *testing.T) {
		r := NewRouter()
		n := r.Namespace("api")

		n.Use(dummyMiddleware)

		if len(n.n.mws) != 1 {
			t.Fatal("didn't create middleware appropriately")
		}

		got := n.n.mws[0]
		want := dummyMiddleware

		if got != want {
			t.Errorf("got middleware %v, but want %v", got, want)
		}
	})

	t.Run("able to add many middleware in the same call", func(t *testing.T) {
		r := NewRouter()
		n := r.Namespace("api")
		n.Use(dummyMiddleware, dummyMiddleware, dummyMiddleware)

		if len(n.n.mws) != 3 {
			t.Errorf("expected to get 3 middlewares, but got %d", len(n.n.mws))
		}
	})

	t.Run("able to add middleware to a specific path from the namespace", func(t *testing.T) {
		r := NewRouter()
		n := r.Namespace("api")
		n.Use("/v1", dummyMiddleware)

		got := reflect.ValueOf(n.namespace("v1").n.mws[0]).Pointer()
		want := reflect.ValueOf(dummyMiddleware).Pointer()

		if got != want {
			t.Errorf("got %v, but want %v", got, want)
		}
	})

	t.Run("UseFunc able to add func as middleware", func(t *testing.T) {
		r := NewRouter()
		n := r.Namespace("api")

		dummyMiddlewareFunc := func(w ResponseWriter, r *Request, next NextMiddlewareCaller) {}

		n.UseFunc(dummyMiddlewareFunc)

		got := reflect.ValueOf(n.n.mws[0]).Pointer()
		want := reflect.ValueOf(dummyMiddlewareFunc).Pointer()

		if got != want {
			t.Errorf("got %#v, but want %#v", got, want)
		}
	})
}

func TestRouter(t *testing.T) {

	router := NewRouter()

	router.Get("/greet", &mockHandler{
		OnHandleFunc: func(w ResponseWriter, r *Request) {
			fmt.Fprint(w, `Hello, Requester`)
		},
	})

	ns := router.Namespace("api")

	ns.Use(&mockMiddleware{
		InterceptFunc: func(w ResponseWriter, r *Request, next NextMiddlewareCaller) {
			if r.Method != http.MethodPost {
				next()
				return
			}
			if r.Header.Get("Content-Type") == "" {
				next(errors.New("Missing content-type in header"))
				return
			}
			next()
		},
	})

	ns.GetFunc("/users/{id}", func(w ResponseWriter, r *Request) {
		fmt.Fprint(w, r.Params()["id"])
	})

	ns.PostFunc("/users/{id}", func(w ResponseWriter, r *Request) {
		payload, _ := io.ReadAll(r.Body)
		fmt.Fprint(w, string(payload))
	})

	if ns == nil {
		t.Fatal("didn't get the namespace")
	}

	router.Use("/admin", &mockMiddleware{
		InterceptFunc: func(w ResponseWriter, r *Request, next NextMiddlewareCaller) {
			if _, ok := r.Header["Authorization"]; !ok {
				next(errors.New("Missing authorization in header"))
				return
			}
			next()
		},
	})

	router.Get("/admin/users", &mockHandler{
		OnHandleFunc: func(w ResponseWriter, r *Request) {
			fmt.Fprint(w, `[]`)
		},
	})

	router.Use(&mockMiddlewareErrorHandler{
		HandleFunc: func(w ResponseWriter, r *Request, e error) {
			switch e.Error() {
			case "Missing authorization in header", "Missing content-type in header":
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprint(w, e.Error())
			default:
				w.WriteHeader(http.StatusInternalServerError)
			}
		},
	})

	t.Run(`GET /greet returns status 200 and expected body`, func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, newDummyURI("/greet"), nil)
		response := httptest.NewRecorder()

		router.ServeHTTP(response, request)

		assertStatus(t, response, http.StatusOK)
		assertBody(t, response, `Hello, Requester`)
	})

	t.Run(`GET /api/users/1 returns status 200 and expected body`, func(t *testing.T) {
		request, _ := http.NewRequest(http.MethodGet, newDummyURI("/api/users/1"), nil)
		response := httptest.NewRecorder()

		router.ServeHTTP(response, request)

		assertStatus(t, response, http.StatusOK)
		assertBody(t, response, `1`)
	})

	t.Run("GET /admin/users returns status 200 and expected body", func(t *testing.T) {

		req, _ := http.NewRequest(MethodGet, newDummyURI("/admin/users"), nil)
		req.Header.Add("Authorization", "[Auth Token]")
		res := httptest.NewRecorder()

		router.ServeHTTP(res, req)

		assertStatus(t, res, http.StatusOK)
		assertBody(t, res, `[]`)
	})

	t.Run("GET /admin/users returns status 400 and expected body", func(t *testing.T) {
		req, _ := http.NewRequest(MethodGet, newDummyURI("/admin/users"), nil)
		res := httptest.NewRecorder()

		router.ServeHTTP(res, req)

		assertStatus(t, res, http.StatusBadRequest)
		assertBody(t, res, "Missing authorization in header")
	})

	t.Run("POST /api/users/3 returns status 400 because middleware treatment", func(t *testing.T) {
		req, _ := http.NewRequest(MethodPost, newDummyURI("/api/users/3"), nil)
		res := httptest.NewRecorder()

		router.ServeHTTP(res, req)

		assertStatus(t, res, http.StatusBadRequest)
		assertBody(t, res, "Missing content-type in header")
	})
}

func BenchmarkRouterMath(b *testing.B) {
	r := NewRouter()
	r.All("/", dummyHandler)
	r.All("/index", dummyHandler)
	r.All("/home", dummyHandler)
	r.All("/about", dummyHandler)
	r.All("/contact", dummyHandler)
	r.All("/robots.txt", dummyHandler)
	r.All("/products/", dummyHandler)
	r.All("/products/{id}", dummyHandler)
	r.All("/products/{id}/image.jpg", dummyHandler)
	r.All("/admin", dummyHandler)
	r.All("/admin/products/", dummyHandler)
	r.Post("/admin/products", dummyHandler)
	r.Put("/admin/products", dummyHandler)
	r.Delete("/admin/products", dummyHandler)

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

	path, _ = parseNamespace(path)
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

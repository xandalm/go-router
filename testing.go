package router

import (
	"net/http/httptest"
	"reflect"
	"testing"
)

type stubHandler struct{}

func (h *stubHandler) ServeHTTP(w ResponseWriter, r *Request) {
}

type mockHandler struct {
	lastParams   Params
	OnHandleFunc func(ResponseWriter, *Request)
}

func (h *mockHandler) ServeHTTP(w ResponseWriter, r *Request) {
	h.lastParams = r.Params()
	h.OnHandleFunc(w, r)
}

type stubMiddleware struct {
}

func (m *stubMiddleware) Intercept(w ResponseWriter, r *Request, next NextMiddlewareCaller) {
}

type spyMiddleware struct {
	intercepted bool
}

func (m *spyMiddleware) Intercept(w ResponseWriter, r *Request, next NextMiddlewareCaller) {
	m.intercepted = true
	next()
}

type mockMiddleware struct {
	InterceptFunc func(w ResponseWriter, r *Request, next NextMiddlewareCaller)
}

func (m *mockMiddleware) Intercept(w ResponseWriter, r *Request, next NextMiddlewareCaller) {
	m.InterceptFunc(w, r, next)
}

func newDummyURI(path string) string {
	return "http://site.com" + path
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

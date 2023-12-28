package router

import (
	"net/http"
)

type RouteHandler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}

type RouteHandlerFunc func(http.ResponseWriter, *http.Request)

func (f RouteHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f(w, r)
}

type routerEntry struct {
	h      RouteHandler
	method string
}

type Router struct {
	m map[string]routerEntry
}

func (ro *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	path := r.URL.Path

	e := ro.m[path]

	e.h.ServeHTTP(w, r)
}

func (ro *Router) register(pattern string, handler RouteHandler, method string) {
	if pattern == "" {
		panic("router: invalid pattern")
	}

	if handler == nil {
		panic("router: nil handler")
	}

	if ro.m == nil {
		ro.m = make(map[string]routerEntry)
	}

	e := routerEntry{
		h:      handler,
		method: method,
	}

	ro.m[pattern] = e
}

func (ro *Router) registerFunc(pattern string, handler func(w http.ResponseWriter, r *http.Request), method string) {
	if handler == nil {
		panic("router: nil handler")
	}
	ro.register(pattern, RouteHandlerFunc(handler), method)
}

// Records the given pattern and handler to handle the corresponding path.
// Use is a generic method correspondent
func (ro *Router) Use(pattern string, handler RouteHandler) {
	ro.register(pattern, handler, "")
}

// Instead Use method, this method get a handler as a func.
// And wrap it, to act like a RouteHandler.
func (ro *Router) UseFunc(pattern string, handler func(w http.ResponseWriter, r *http.Request)) {
	ro.registerFunc(pattern, RouteHandlerFunc(handler), "")
}

// Records the given pattern and handler to handle the corresponding path only on GET method.
func (ro *Router) Get(pattern string, handler RouteHandler) {
	ro.register(pattern, handler, http.MethodGet)
}

func (ro *Router) GetFunc(pattern string, handler func(w http.ResponseWriter, r *http.Request)) {
	ro.registerFunc(pattern, handler, http.MethodGet)
}

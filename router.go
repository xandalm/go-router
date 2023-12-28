package router

import (
	"net/http"
)

const (
	MethodAll    = "ALL"
	MethodGet    = http.MethodGet
	MethodPost   = http.MethodPost
	MethodPut    = http.MethodPut
	MethodDelete = http.MethodDelete
)

type RouteHandler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}

type RouteHandlerFunc func(http.ResponseWriter, *http.Request)

func (f RouteHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f(w, r)
}

type routerEntry struct {
	mh map[string]RouteHandler
}

type Router struct {
	m map[string]routerEntry
}

func (ro *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	path := r.URL.Path

	e := ro.m[path]

	h, ok := e.mh[r.Method]
	if !ok {
		if h, ok := e.mh["ALL"]; ok {
			h.ServeHTTP(w, r)
			return
		}
	}
	h.ServeHTTP(w, r)
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

	e, ok := ro.m[pattern]
	if ok {
		if _, ok := e.mh[method]; ok {
			panic("router: multiple registration into " + pattern)
		}
	} else {
		e = routerEntry{
			mh: make(map[string]RouteHandler),
		}
	}

	e.mh[method] = handler

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
	ro.register(pattern, handler, MethodAll)
}

// Instead Use method, this method get a handler as a func.
// And wrap it, to act like a RouteHandler.
func (ro *Router) UseFunc(pattern string, handler func(w http.ResponseWriter, r *http.Request)) {
	ro.registerFunc(pattern, RouteHandlerFunc(handler), MethodAll)
}

// Records the given pattern and handler to handle the corresponding path only on GET method.
func (ro *Router) Get(pattern string, handler RouteHandler) {
	ro.register(pattern, handler, MethodGet)
}

func (ro *Router) GetFunc(pattern string, handler func(w http.ResponseWriter, r *http.Request)) {
	ro.registerFunc(pattern, handler, MethodGet)
}

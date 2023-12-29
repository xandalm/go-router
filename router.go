package router

import (
	"net/http"
	"sync"
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

func NotFoundHandler() RouteHandler {
	return RouteHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
}

type Router struct {
	mu sync.RWMutex
	m  map[string]routerEntry
}

func NewRouter() *Router {
	return &Router{}
}

func (ro *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	_, h := ro.Handler(r)
	h.ServeHTTP(w, r)
}

func (ro *Router) Handler(r *http.Request) (p string, h RouteHandler) {
	ro.mu.RLock()
	defer ro.mu.RUnlock()

	path := r.URL.Path

	e := ro.m[path]

	h, ok := e.mh[r.Method]
	if ok {
		p = path
		return
	}
	if _, ok := e.mh[MethodAll]; ok {
		h = e.mh[MethodAll]
		p = path
		return
	}
	return "", NotFoundHandler()
}

func (ro *Router) register(pattern string, handler RouteHandler, method string) {
	ro.mu.Lock()
	defer ro.mu.Unlock()

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

// Similar to Use method, but this method get a handler as a func.
// And wrap it, to act like a RouteHandler.
func (ro *Router) UseFunc(pattern string, handler func(w http.ResponseWriter, r *http.Request)) {
	ro.registerFunc(pattern, RouteHandlerFunc(handler), MethodAll)
}

// Records the given pattern and handler to handle the corresponding path only on GET method.
func (ro *Router) Get(pattern string, handler RouteHandler) {
	ro.register(pattern, handler, MethodGet)
}

// Similar to Get method, but this method get a handler as a func.
// And wrap it, to act like a RouteHandler.
func (ro *Router) GetFunc(pattern string, handler func(w http.ResponseWriter, r *http.Request)) {
	ro.registerFunc(pattern, handler, MethodGet)
}

// Records the given pattern and handler to handle the corresponding path only on POST method.
func (ro *Router) Post(pattern string, handler RouteHandler) {
	ro.register(pattern, handler, MethodPost)
}

// Similar to Post method, but this method get a handler as a func.
// And wrap it, to act like a RouteHandler.
func (ro *Router) PostFunc(pattern string, handler func(w http.ResponseWriter, r *http.Request)) {
	ro.registerFunc(pattern, handler, MethodPost)
}

// Records the given pattern and handler to handle the corresponding path only on PUT method.
func (ro *Router) Put(pattern string, handler RouteHandler) {
	ro.register(pattern, handler, MethodPut)
}

// Similar to Put method, but this method get a handler as a func.
// And wrap it, to act like a RouteHandler.
func (ro *Router) PutFunc(pattern string, handler func(w http.ResponseWriter, r *http.Request)) {
	ro.registerFunc(pattern, handler, MethodPut)
}

// Records the given pattern and handler to handle the corresponding path only on DELETE method.
func (ro *Router) Delete(pattern string, handler RouteHandler) {
	ro.register(pattern, handler, MethodDelete)
}

// Similar to Delete method, but this method get a handler as a func.
// And wrap it, to act like a RouteHandler.
func (ro *Router) DeleteFunc(pattern string, handler func(w http.ResponseWriter, r *http.Request)) {
	ro.registerFunc(pattern, handler, MethodDelete)
}

package router

import "net/http"

type RouteHandler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}

type Router struct {
	m map[string]RouteHandler
}

func (ro *Router) Use(pattern string, handler RouteHandler) {
	if pattern == "" {
		panic("router: invalid pattern")
	}

	if handler == nil {
		panic("router: nil handler")
	}

	if ro.m == nil {
		ro.m = make(map[string]RouteHandler)
	}
	ro.m[pattern] = handler
}

func (ro *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	path := r.URL.Path

	h := ro.m[path]

	h.ServeHTTP(w, r)
}

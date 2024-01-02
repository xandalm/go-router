package router

import (
	"net/http"
	"regexp"
	"strings"
	"sync"
)

const (
	MethodAll    = "ALL"
	MethodGet    = http.MethodGet
	MethodPost   = http.MethodPost
	MethodPut    = http.MethodPut
	MethodDelete = http.MethodDelete
)

type ResponseWriter http.ResponseWriter

type RouteHandler interface {
	ServeHTTP(ResponseWriter, *Request)
}

type RouteHandlerFunc func(ResponseWriter, *Request)

func (f RouteHandlerFunc) ServeHTTP(w ResponseWriter, r *Request) {
	f(w, r)
}

type routerEntry struct {
	pattern string
	re      *regexp.Regexp
	mh      map[string]RouteHandler
}

type notFoundHandler struct {
}

func (h *notFoundHandler) ServeHTTP(w ResponseWriter, r *Request) {
	w.WriteHeader(http.StatusNotFound)
}

var NotFoundHandler = &notFoundHandler{}

type Router struct {
	mu   sync.RWMutex
	m    map[string]routerEntry
	host bool
}

func NewRouter() *Router {
	return &Router{}
}

func (ro *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	_, h, params := ro.Handler(r)
	h.ServeHTTP(w, &Request{params: params, Request: r})
}

func (ro *Router) Handler(r *http.Request) (p string, h RouteHandler, params Params) {

	host := r.URL.Host
	path := r.URL.Path

	var e *routerEntry

	if ro.host {
		e = ro.match(host + path)
	}

	if e == nil {
		e = ro.match(path)
	}

	if e == nil {
		return "", NotFoundHandler, nil
	}

	h = e.mh[r.Method]

	if h == nil {
		h = e.mh[MethodAll]
	}

	if h != nil {
		p = e.pattern
		matches := e.re.FindStringSubmatch(path)
		params = make(Params)

		for i, tag := range e.re.SubexpNames() {
			if i != 0 && tag != "" {
				params[tag] = matches[i]
			}
		}
		return
	}

	return "", NotFoundHandler, nil
}

func (ro *Router) match(path string) *routerEntry {
	ro.mu.RLock()
	defer ro.mu.RUnlock()

	// Check exactly match
	e, ok := ro.m[path]
	if ok && e.re.MatchString(path) {
		return &e
	}

	for _, e := range ro.m {
		if e.re.MatchString(path) {
			return &e
		}
	}

	return nil
}

func createRegExp(pattern string) *regexp.Regexp {

	builder := strings.Builder{}

	builder.WriteRune('^')

	paramsSeeker := regexp.MustCompile(`(\/\{[^\/]+\})`)
	builder.WriteString(paramsSeeker.ReplaceAllStringFunc(pattern, func(m string) string {
		return "/(?P<" + m[2:len(m)-1] + ">[^/]+)"
	}))

	builder.WriteString("$")

	return regexp.MustCompile(strings.ReplaceAll(builder.String(), "/", `\/`))
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
			pattern: pattern,
			re:      createRegExp(pattern),
			mh:      make(map[string]RouteHandler),
		}
	}

	e.mh[method] = handler

	ro.m[pattern] = e

	ro.host = pattern[0] != '/'
}

func (ro *Router) registerFunc(pattern string, handler func(w ResponseWriter, r *Request), method string) {
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
func (ro *Router) UseFunc(pattern string, handler func(w ResponseWriter, r *Request)) {
	ro.registerFunc(pattern, RouteHandlerFunc(handler), MethodAll)
}

// Records the given pattern and handler to handle the corresponding path only on GET method.
func (ro *Router) Get(pattern string, handler RouteHandler) {
	ro.register(pattern, handler, MethodGet)
}

// Similar to Get method, but this method get a handler as a func.
// And wrap it, to act like a RouteHandler.
func (ro *Router) GetFunc(pattern string, handler func(w ResponseWriter, r *Request)) {
	ro.registerFunc(pattern, handler, MethodGet)
}

// Records the given pattern and handler to handle the corresponding path only on POST method.
func (ro *Router) Post(pattern string, handler RouteHandler) {
	ro.register(pattern, handler, MethodPost)
}

// Similar to Post method, but this method get a handler as a func.
// And wrap it, to act like a RouteHandler.
func (ro *Router) PostFunc(pattern string, handler func(w ResponseWriter, r *Request)) {
	ro.registerFunc(pattern, handler, MethodPost)
}

// Records the given pattern and handler to handle the corresponding path only on PUT method.
func (ro *Router) Put(pattern string, handler RouteHandler) {
	ro.register(pattern, handler, MethodPut)
}

// Similar to Put method, but this method get a handler as a func.
// And wrap it, to act like a RouteHandler.
func (ro *Router) PutFunc(pattern string, handler func(w ResponseWriter, r *Request)) {
	ro.registerFunc(pattern, handler, MethodPut)
}

// Records the given pattern and handler to handle the corresponding path only on DELETE method.
func (ro *Router) Delete(pattern string, handler RouteHandler) {
	ro.register(pattern, handler, MethodDelete)
}

// Similar to Delete method, but this method get a handler as a func.
// And wrap it, to act like a RouteHandler.
func (ro *Router) DeleteFunc(pattern string, handler func(w ResponseWriter, r *Request)) {
	ro.registerFunc(pattern, handler, MethodDelete)
}

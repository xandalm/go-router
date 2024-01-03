package router

import (
	"net/http"
	"net/url"
	"path"
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

var NotFoundHandler = RouteHandlerFunc(func(w ResponseWriter, r *Request) {
	w.WriteHeader(http.StatusNotFound)
})

type redirectHandler struct {
	url  string
	code int
}

func (rh *redirectHandler) ServeHTTP(w ResponseWriter, r *Request) {
	http.Redirect(w, r.Request, rh.url, rh.code)
}

func RedirectHandler(url string, code int) RouteHandler {
	return &redirectHandler{url, code}
}
func cleanPath(p string) string {
	if p == "" {
		return "/"
	}

	if p[0] != '/' {
		p = "/" + p
	}
	np := path.Clean(p)

	if p[len(p)-1] == '/' && np != "/" {
		if len(p) == len(np)+1 && strings.HasPrefix(p, np) {
			np = p
		} else {
			np += "/"
		}
	}

	return np
}

type Router struct {
	mu   sync.RWMutex
	m    map[string]*routerEntry
	sm   map[string]*routerEntry
	um   map[string]*routerEntry
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
	path := cleanPath(r.URL.Path)

	p, h, params = ro.handler(host, path, r.Method)

	if h != nil {

		if path != r.URL.Path {
			u := &url.URL{Path: path, RawQuery: r.URL.RawQuery}
			return u.Path, RedirectHandler(u.String(), http.StatusMovedPermanently), nil
		}

		return
	}

	if newPath, ok := ro.shouldRedirectToSlashPath(path); ok {
		u := &url.URL{Path: newPath, RawQuery: r.URL.RawQuery}
		return u.Path, RedirectHandler(u.String(), http.StatusMovedPermanently), nil
	}

	if newPath, ok := ro.shouldRedirectToUnslashPath(path); ok {
		u := &url.URL{Path: newPath, RawQuery: r.URL.RawQuery}
		return u.Path, RedirectHandler(u.String(), http.StatusMovedPermanently), nil
	}

	return "", NotFoundHandler, nil
}

func (ro *Router) handler(host, path, method string) (p string, h RouteHandler, params Params) {
	var e *routerEntry

	if ro.host {
		e = ro.match(host + path)
	}

	if e == nil {
		e = ro.match(path)
	}

	if e == nil {
		return "", nil, nil
	}

	h = e.mh[method]

	if h == nil {
		h = e.mh[MethodAll]
		if h == nil {
			return "", nil, nil
		}
	}

	matches := e.re.FindStringSubmatch(path)
	params = make(Params)

	for i, tag := range e.re.SubexpNames() {
		if i != 0 && tag != "" {
			params[tag] = matches[i]
		}
	}
	return e.pattern, h, params
}

func (ro *Router) shouldRedirectToUnslashPath(path string) (string, bool) {
	ro.mu.RLock()
	defer ro.mu.RUnlock()

	l := len(path)
	if path[l-1] != '/' {
		return "", false
	}

	path = path[:l-1]

	if _, ok := ro.um[path]; ok {
		return path, true
	}

	for _, e := range ro.um {
		if e.re.MatchString(path) {
			return path, true
		}
	}

	return "", false

}

func (ro *Router) shouldRedirectToSlashPath(path string) (string, bool) {
	ro.mu.RLock()
	defer ro.mu.RUnlock()

	if path[len(path)-1] == '/' {
		return "", false
	}

	path = path + "/"

	if _, ok := ro.sm[path]; ok {
		return path, true
	}

	for _, e := range ro.sm {
		if e.re.MatchString(path) {
			return path, true
		}
	}

	return "", false
}

func (ro *Router) match(path string) *routerEntry {
	ro.mu.RLock()
	defer ro.mu.RUnlock()

	// Check exactly match
	e, ok := ro.m[path]
	if ok && e.re.MatchString(path) {
		return e
	}

	for _, e := range ro.m {
		if e.re.MatchString(path) {
			return e
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
		ro.m = make(map[string]*routerEntry)
	}

	e, ok := ro.m[pattern]
	if ok {
		if _, ok := e.mh[method]; ok {
			panic("router: multiple registration into " + pattern)
		}
	} else {
		e = &routerEntry{
			pattern: pattern,
			re:      createRegExp(pattern),
			mh:      make(map[string]RouteHandler),
		}
	}

	e.mh[method] = handler

	ro.m[pattern] = e

	if pattern[len(pattern)-1] == '/' {
		if ro.sm == nil {
			ro.sm = make(map[string]*routerEntry)
		}
		ro.sm[e.pattern] = e
	} else {
		if ro.um == nil {
			ro.um = make(map[string]*routerEntry)
		}
		ro.um[e.pattern] = e
	}

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

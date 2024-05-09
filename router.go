package router

import (
	"net"
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

// An Adapter to allow the use of functions as HTTP handlers.
type RouteHandlerFunc func(ResponseWriter, *Request)

func (f RouteHandlerFunc) ServeHTTP(w ResponseWriter, r *Request) {
	f(w, r)
}

type routerEntry struct {
	pattern string
	re      *regexp.Regexp
	mh      map[string]RouteHandler
}

// Holds a simple request handler that replies HTTP 404 status
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

func stripHostPort(host string) string {
	if !strings.Contains(host, ":") {
		return host
	}
	host, _, err := net.SplitHostPort(host)
	if err != nil {
		return host
	}
	return host
}

// Like to standard ServeMux, it's a HTTP request multiplexer.
// Have similar characteristics, however Router brings the
// possibility to handle params that can be exposed in patterns.
//
// One parameterized pattern can be registered with a it's name
// rounded by brackets, that is /customers/{id}.
type Router struct {
	mu   sync.RWMutex
	ns   map[string]*routerNamespace
	e    *routerEntry // handle with "/" (the root)
	host bool
}

func NewRouter() *Router {
	return &Router{}
}

// Dispatches the request to the handler whose pattern most closely matches the request URL.
func (ro *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI == "*" {
		if r.ProtoAtLeast(1, 1) {
			w.Header().Set("Connection", "close")
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	h, _, params := ro.Handler(r)
	h.ServeHTTP(w, &Request{params: params, Request: r})
}

// Returns the handler for the given request accordingly to the request characteristics
// (r.Method, r.Host and r.URL.Path), it will never be nil. If the request path is not in
// its canonical form, the result handler will be an handler that redirects to the canonical
// path.
//
// Handler also returns the registered pattern that matches the request, or will match, in
// case of a redirect handler.
//
// Finally, also returns identified params from the given request path, if registered pattern
// matches one.
//
// To the unrecognizable request path it gives a not found handler, empty pattern and nil params.
func (ro *Router) Handler(r *http.Request) (h RouteHandler, p string, params Params) {

	var host string
	var path string

	if r.Method == http.MethodConnect {
		host = r.URL.Host
		path = r.URL.Path
	} else {
		host = stripHostPort(r.Host)
		path = cleanPath(r.URL.Path)
	}

	p, h, params = ro.handler(host, path, r.Method)

	if h != nil {

		if path != "/" && path != r.URL.Path {
			u := &url.URL{Path: path, RawQuery: r.URL.RawQuery}
			return RedirectHandler(u.String(), http.StatusMovedPermanently), u.Path, nil
		}

		return
	}

	if newPath, ok := ro.shouldRedirectToSlashPath(host, path); ok {
		u := &url.URL{Path: newPath, RawQuery: r.URL.RawQuery}
		return RedirectHandler(u.String(), http.StatusMovedPermanently), u.Path, nil
	}

	if newPath, ok := ro.shouldRedirectToUnslashPath(host, path); ok {
		u := &url.URL{Path: newPath, RawQuery: r.URL.RawQuery}
		return RedirectHandler(u.String(), http.StatusMovedPermanently), u.Path, nil
	}

	return NotFoundHandler, "", nil
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

func (ro *Router) shouldRedirectToUnslashPath(host, path string) (string, bool) {
	ro.mu.RLock()
	defer ro.mu.RUnlock()

	if path[len(path)-1] != '/' {
		return "", false
	}

	p := []string{path, host + path}

	for _, c := range p {
		ps := c[:len(c)-1]
		n, _ := closer(ro.ns, parseNamespace(ps))
		if n != nil && n.eu != nil && n.eu.re.MatchString(ps) {
			return ps, true
		}
	}

	return "", false

}

func (ro *Router) shouldRedirectToSlashPath(host, path string) (string, bool) {
	ro.mu.RLock()
	defer ro.mu.RUnlock()

	if path[len(path)-1] == '/' {
		return "", false
	}

	p := []string{path, host + path}

	for _, c := range p {
		ps := c + "/"
		n, _ := closer(ro.ns, parseNamespace(ps))
		if n != nil && n.es != nil && n.es.re.MatchString(ps) {
			return ps, true
		}
	}

	return "", false
}

func (ro *Router) match(path string) *routerEntry {
	ro.mu.RLock()
	defer ro.mu.RUnlock()

	if path == "/" {
		return ro.e
	}

	n, _ := closer(ro.ns, strings.TrimPrefix(path, "/"))

	if n == nil {
		return nil
	}

	if n.eu != nil && n.eu.re.MatchString(path) {
		return n.eu
	}

	if n.es != nil && n.es.re.MatchString(path) {
		return n.es
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

	str := regexp.MustCompile(`\/|\.`).ReplaceAllStringFunc(
		builder.String(),
		func(s string) string {
			if s == "/" {
				return `\/`
			}
			return `\.`
		},
	)

	return regexp.MustCompile(str)
}

var PatternValidator = regexp.MustCompile(`^((?:\w+\.)+\w+)?((?:\/(?:\w+|(?:\{\w+\}))+)*(?:\/(?:\w*(?:\.\w+)*)?)?)?$`)

func isValidPattern(p string) bool {

	if PatternValidator == nil {
		panic("router: nil pattern validator")
	}

	if p == "" {
		return false
	}

	return !PatternValidator.MatchString(p)
}

func (ro *Router) register(pattern string, handler RouteHandler, method string) {
	ro.mu.Lock()
	defer ro.mu.Unlock()

	if isValidPattern(pattern) {
		panic("router: invalid pattern")
	}

	if handler == nil {
		panic("router: nil handler")
	}

	if pattern == "/" {
		// handle for http://example.url and http://example.url/
		if ro.e != nil {
			panic("router: multiple registration into " + pattern)
		}
		ro.e = &routerEntry{
			pattern: pattern,
			re:      regexp.MustCompile(`^\/?$`),
			mh: map[string]RouteHandler{
				method: handler,
			},
		}
		return
	}

	var n *routerNamespace
	if pattern[0] == '/' {
		n = ro.namespace(pattern[1:])
	} else {
		ro.host = true
		n = ro.namespace(pattern)
	}

	var holdEntry **routerEntry
	if pattern[len(pattern)-1] == '/' {
		holdEntry = &n.es
	} else {
		holdEntry = &n.eu
	}

	if *holdEntry != nil {
		entry := **holdEntry
		if _, ok := entry.mh[method]; ok {
			panic("router: multiple registration into " + pattern)
		}
		entry.mh[method] = handler
	} else {
		*holdEntry = &routerEntry{
			pattern: pattern,
			re:      createRegExp(pattern),
			mh: map[string]RouteHandler{
				method: handler,
			},
		}
	}
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

type routerNamespace struct {
	r      *Router
	p      *routerNamespace // parent
	es, eu *routerEntry
	ns     map[string]*routerNamespace
}

func (na *routerNamespace) namespace(name string) *routerNamespace {

	if na.ns == nil {
		na.ns = map[string]*routerNamespace{}
	}

	n, path := closer(na.ns, name)

	if path == name {
		return n
	}
	name = strings.TrimPrefix(name, path+"/")

	// new node (nn)
	nn := &routerNamespace{
		r:  na.r,
		p:  na,
		ns: map[string]*routerNamespace{},
	}

	var ns map[string]*routerNamespace
	if n == nil {
		// hold router children (namespace list from this level)
		ns = na.ns
	} else {
		// hold falling node children (namespace list from this level)
		ns = n.ns
		// set falling node parent to be parent of the new node
		nn.p = n.p
		// set new node as parent of the falling node
		n.p = nn
	}

	for k, v := range ns {
		last := strings.TrimPrefix(k, name+"/")
		if last == k {
			continue
		}
		delete(ns, k)
		v.p = nn
		nn.ns[last] = v
	}

	ns[name] = nn // ignoring slash

	return nn
}

func (na *routerNamespace) Namespace(name string) *routerNamespace {

	return na.namespace(name)
}

func closer(ns map[string]*routerNamespace, name string) (n *routerNamespace, path string) {
	subnames := strings.Split(name, "/")

	var acc string
	var before string
	for _, name := range subnames {
		before = acc
		acc += name
		if found, ok := ns[acc]; ok { // Exact match
			n = found
			ns = n.ns // next level
			path += acc + "/"
			acc = ""
		} else {
			if found, ok := ns[before+"{}"]; ok { // Has param that can handle with path
				n = found
				ns = n.ns // next level
				path += acc + "{}/"
				acc = ""
			} else {
				acc += "/"
			}
		}
	}

	if path != "" && path != name {
		path = path[:len(path)-1]
	}

	return
}

const ErrParamAsNamespace = "the given namespace starts with param"

// This function checks for isolated param as namespace ocurrence.
// If it happens then it will panic.
// Otherwise will replace params into generalized params
// As example, the path:
//
//	"/some/path/{PARAM_NAME}"
//
// Will be normalized to:
//
//	"/some/path/{any}"
//
// Finally returning the parsed name.
func parseNamespace(name string) string {
	name = strings.TrimPrefix(name, "/")
	name = strings.TrimSuffix(name, "/")
	if regexp.MustCompile(`^\{[^\/]+\}`).MatchString(name) {
		panic(ErrParamAsNamespace)
	}
	name = regexp.MustCompile(`\{[^\/]+\}`).ReplaceAllString(name, "{}")
	return name
}

func (ro *Router) namespace(name string) *routerNamespace {

	name = parseNamespace(name)

	if ro.ns == nil {
		ro.ns = map[string]*routerNamespace{}
	}

	n, path := closer(ro.ns, name)

	if path == name {
		return n
	}
	name = strings.TrimPrefix(name, path+"/")

	// new node (nn)
	nn := &routerNamespace{
		r:  ro,
		ns: map[string]*routerNamespace{},
	}

	var ns map[string]*routerNamespace
	if n == nil {
		// hold router children (namespace list from this level)
		ns = ro.ns
	} else {
		// hold falling node children (namespace list from this level)
		ns = n.ns
		// set falling node parent to be parent of the new node
		nn.p = n.p
		// set new node as parent of the falling node
		n.p = nn
	}

	for k, v := range ns {
		last := strings.TrimPrefix(k, name+"/")
		if last == k {
			continue
		}
		delete(ns, k)
		v.p = nn
		nn.ns[last] = v
	}

	ns[name] = nn // ignoring slash

	return nn
}

func (ro *Router) Namespace(name string) *routerNamespace {
	ro.mu.Lock()
	defer ro.mu.Unlock()

	return ro.namespace(name)
}

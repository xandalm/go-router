package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"runtime/debug"
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

const ErrNamespaceStartsWithParam = "the given namespace starts with param"

const (
	PanicMsgInvalidPattern      = "router: invalid pattern"
	PanicMsgInvalidNamespace    = "router: invalid namespace"
	PanicMsgEmptyHandler        = "router: nil handler"
	PanicMsgMissingHandler      = "router: missing handler"
	PanicMsgEndpointDuplication = "router: endpoint duplication"
)

type ResponseWriter http.ResponseWriter

type Handler interface {
	ServeHTTP(ResponseWriter, *Request)
}

type NextMiddlewareCaller func(...error)

type Middleware interface {
	Intercept(ResponseWriter, *Request, NextMiddlewareCaller)
}

type MiddlewareErrorHandler interface {
	Handle(ResponseWriter, *Request, error)
}

type MiddlewareFunc func(ResponseWriter, *Request, NextMiddlewareCaller)

func (f MiddlewareFunc) Intercept(w ResponseWriter, r *Request, next NextMiddlewareCaller) {
	f(w, r, next)
}

type MiddlewareErrorHandlerFunc func(ResponseWriter, *Request, error)

func (f MiddlewareErrorHandlerFunc) Handle(w ResponseWriter, r *Request, err error) {
	f(w, r, err)
}

// An Adapter to allow the use of functions as HTTP handlers.
type HandlerFunc func(ResponseWriter, *Request)

func (f HandlerFunc) ServeHTTP(w ResponseWriter, r *Request) {
	f(w, r)
}

type notFoundHandler struct{}

func (h *notFoundHandler) ServeHTTP(w ResponseWriter, r *Request) {
	w.WriteHeader(http.StatusNotFound)
}

// Holds a simple request handler that replies HTTP 404 status
var NotFoundHandler = &notFoundHandler{}

type redirectHandler struct {
	url  string
	code int
}

func (rh *redirectHandler) ServeHTTP(w ResponseWriter, r *Request) {
	http.Redirect(w, r.Request, rh.url, rh.code)
}

// Creates a redirect handler
func RedirectHandler(url string, code int) Handler {
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

func createRegExp(pattern string) *regexp.Regexp {

	builder := strings.Builder{}

	builder.WriteRune('^')

	builder.WriteString(paramsSeeker.ReplaceAllStringFunc(pattern, func(m string) string {
		return "(?P<" + m[1:len(m)-1] + ">[^/]+)"
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

var patternValidator = regexp.MustCompile(`^((?:\w+\.)+\w+)?((?:\/(?:\w+|(?:\{\w+\}))+)*(?:\/(?:\w*(?:\.\w+)*)?)?)?$`)
var namespaceValidator = regexp.MustCompile(`^((?:\w+\.)+\w+)?((?:\/?(?:\w+|(?:\{\w+\}))+)*(?:\/(?:\w*(?:\.\w+)*)?)?)?$`)

func isValidPattern(p string) bool {

	if p == "" {
		return false
	}

	return patternValidator.MatchString(p)
}

func isValidNamespace(p string) bool {

	if p == "" || p[0] == '/' {
		return false
	}

	return namespaceValidator.MatchString(p)
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

var paramsSeeker = regexp.MustCompile(`\{[^\/]+\}`)

// This function trims prefix and suffix bars from the namespace.
// Also normalize the params names to be a generalized param.
// As example, the path:
//
//	"/some/path/{PARAM_NAME}"
//
// Will be normalized to:
//
//	"/some/path/{}"
//
// Finally returns the parsed name.
func parseNamespace(name string) (string, []string) {
	name = strings.TrimPrefix(name, "/")
	name = strings.TrimSuffix(name, "/")

	var params []string
	name = paramsSeeker.ReplaceAllStringFunc(name, func(s string) string {
		params = append(params, s)
		return "{}"
	})
	return name, params
}

type routerEntry struct {
	pattern string
	re      *regexp.Regexp
	mh      map[string]Handler
}

type mwError struct {
	err   error
	stack string
}

// Like to standard ServeMux, it's a HTTP request multiplexer.
// Have similar characteristics, however Router brings the
// possibility to handle params that can be exposed in patterns.
//
// The pattern can have params, which are added with its name
// rounded by brackets, like "/customers/{id}".
type Router struct {
	mu   sync.RWMutex
	ns   map[string]*routerNamespace
	mws  []Middleware
	meh  MiddlewareErrorHandler
	e    *routerEntry // handle with "/" (the root)
	host bool
}

func NewRouter() *Router {
	return &Router{
		ns: map[string]*routerNamespace{},
	}
}

// Dispatches the request to the handler whose pattern matches the request URL.
func (ro *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI == "*" {
		if r.ProtoAtLeast(1, 1) {
			w.Header().Set("Connection", "close")
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	h, p, params := ro.Handler(r)
	rr := &Request{params: params, Request: r}
	var errors []mwError
	if errors = ro.crossMiddlewares(p, w, rr); len(errors) > 0 {
		err := errors[0]
		if ro.meh == nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(fmt.Sprintf("Middleware Error: %s\n%s", err.err, err.stack))
		} else {
			ro.meh.Handle(w, rr, err.err)
		}
		return
	}
	h.ServeHTTP(w, rr)
}

func crossMiddlewaresLayer(path []string, ns *map[string]*routerNamespace, mw *[]Middleware, w ResponseWriter, r *Request) chan []mwError {
	iCh := make(chan int, 1)
	errs := []mwError{}

	var l string // layer
	if len(path) > 0 {
		l = path[0]
	}

	if size := len(*mw); size > 0 {
		iCh <- 0
		for loop := true; loop; {
			select {
			case idx := <-iCh:
				if idx >= size {
					loop = false
				} else {
					proceed := new(bool)
					(*mw)[idx].Intercept(
						w,
						r,
						NextMiddlewareCaller(
							func(e ...error) {
								*proceed = true
								if len(e) > 0 {
									stack := debug.Stack()
									c := new(int)
									idx := bytes.IndexFunc(stack, func(r rune) bool {
										if r == '\n' {
											(*c)++
										}
										if *c == 5 {
											return true
										}
										return false
									})
									errs = append(errs, mwError{e[0], string(stack[idx+1:])})
								}
							},
						),
					)
					if len(errs) > 0 {
						loop = false
					} else if *proceed {
						iCh <- idx + 1
					}
				}
			case <-r.Context().Done():
				loop = false
			}
		}
	}
	close(iCh)
	if fwd, ok := (*ns)[l]; ok {
		errs = append(
			errs,
			<-crossMiddlewaresLayer(path[1:], &fwd.ns, &fwd.mws, w, r)...,
		)
	}
	ch := make(chan []mwError, 1)
	ch <- errs
	return ch
}

func (ro *Router) crossMiddlewares(p string, w ResponseWriter, r *Request) []mwError {
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, "/")

	errors := <-crossMiddlewaresLayer(strings.Split(p, "/"), &ro.ns, &ro.mws, w, r)
	return errors
}

// Returns the handler for the given request accordingly to the request characteristics
// (r.Method, r.Host and r.URL.Path), it will never be nil. If the request path is not in
// its canonical form, the result handler will be a handler that redirects to the canonical
// path.
//
// Handler also returns the registered pattern that matches the request, or will match, in
// case of a redirect handler.
//
// Finally, also returns identified params from the given request path, if registered pattern
// matches one.
//
// To the unrecognizable request path it gives a not found handler, empty pattern and nil params.
func (ro *Router) Handler(r *http.Request) (h Handler, p string, params Params) {

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

	if newPath, ok := ro.shouldRedirectToSlashPath(host, path, r.Method); ok {
		u := &url.URL{Path: newPath, RawQuery: r.URL.RawQuery}
		return RedirectHandler(u.String(), http.StatusMovedPermanently), u.Path, nil
	}

	if newPath, ok := ro.shouldRedirectToUnslashPath(host, path, r.Method); ok {
		u := &url.URL{Path: newPath, RawQuery: r.URL.RawQuery}
		return RedirectHandler(u.String(), http.StatusMovedPermanently), u.Path, nil
	}

	return NotFoundHandler, "", nil
}

func (ro *Router) handler(host, path, method string) (p string, h Handler, params Params) {
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

func (ro *Router) shouldRedirectToUnslashPath(host, path, method string) (string, bool) {
	ro.mu.RLock()
	defer ro.mu.RUnlock()

	if path[len(path)-1] != '/' {
		return "", false
	}

	p := []string{path, host + path}

	for _, c := range p {
		ps := c[:len(c)-1]
		name, _ := parseNamespace(ps)
		n, _ := closer(ro.ns, name)
		if n == nil {
			continue
		}
		var entry *routerEntry = n.eu
		if entry == nil {
			continue
		}
		if _, ok := entry.mh[method]; !ok {
			if _, ok := entry.mh[MethodAll]; !ok {
				continue
			}
		}
		if entry.re.MatchString(ps) {
			return ps, true
		}
	}

	return "", false

}

func (ro *Router) shouldRedirectToSlashPath(host, path, method string) (string, bool) {
	ro.mu.RLock()
	defer ro.mu.RUnlock()

	if path[len(path)-1] == '/' {
		return "", false
	}

	p := []string{path, host + path}

	for _, c := range p {
		ps := c + "/"
		name, _ := parseNamespace(ps)
		n, _ := closer(ro.ns, name)
		if n == nil {
			continue
		}
		var entry *routerEntry = n.es
		if entry == nil {
			continue
		}
		if _, ok := entry.mh[method]; !ok {
			if _, ok := entry.mh[MethodAll]; !ok {
				continue
			}
		}
		if entry.re.MatchString(ps) {
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

func (ro *Router) register(pattern string, handler Handler, method string) {
	ro.mu.Lock()
	defer ro.mu.Unlock()

	if !isValidPattern(pattern) {
		panic(PanicMsgInvalidPattern)
	}

	if handler == nil {
		panic(PanicMsgEmptyHandler)
	}

	if pattern == "/" {
		// handle for http://example.url and http://example.url/
		if ro.e != nil {
			if _, ok := ro.e.mh[method]; ok {
				panic(PanicMsgEndpointDuplication)
			}
		}
		ro.e = &routerEntry{
			pattern: pattern,
			re:      regexp.MustCompile(`^\/?$`),
			mh: map[string]Handler{
				method: handler,
			},
		}
		return
	}

	if pattern[0] != '/' {
		ro.host = true
	}

	name, _ := parseNamespace(pattern)
	n := ro.namespace(name)

	var holdEntry **routerEntry
	if pattern[len(pattern)-1] == '/' {
		holdEntry = &n.es
	} else {
		holdEntry = &n.eu
	}

	if *holdEntry != nil {
		entry := **holdEntry
		if _, ok := entry.mh[method]; ok {
			panic(PanicMsgEndpointDuplication)
		}
		entry.mh[method] = handler
		return
	}
	*holdEntry = &routerEntry{
		pattern: pattern,
		re:      createRegExp(pattern),
		mh: map[string]Handler{
			method: handler,
		},
	}
}

func (ro *Router) registerFunc(pattern string, handler func(w ResponseWriter, r *Request), method string) {
	if handler == nil {
		panic(PanicMsgEmptyHandler)
	}
	ro.register(pattern, HandlerFunc(handler), method)
}

// Records the given pattern and handler to handle the corresponding path.
// All is a generic method correspondent
func (ro *Router) All(pattern string, handler Handler) {
	ro.register(pattern, handler, MethodAll)
}

// Similar to All method, but this method get a handler as a func
// and wrap it, to act like a Handler.
func (ro *Router) AllFunc(pattern string, handler func(w ResponseWriter, r *Request)) {
	ro.registerFunc(pattern, HandlerFunc(handler), MethodAll)
}

// Records the given pattern and handler to handle the corresponding path only on GET method.
func (ro *Router) Get(pattern string, handler Handler) {
	ro.register(pattern, handler, MethodGet)
}

// Similar to Get method, but this method get a handler as a func
// and wrap it, to act like a Handler.
func (ro *Router) GetFunc(pattern string, handler func(w ResponseWriter, r *Request)) {
	ro.registerFunc(pattern, handler, MethodGet)
}

// Records the given pattern and handler to handle the corresponding path only on POST method.
func (ro *Router) Post(pattern string, handler Handler) {
	ro.register(pattern, handler, MethodPost)
}

// Similar to Post method, but this method get a handler as a func
// and wrap it, to act like a Handler.
func (ro *Router) PostFunc(pattern string, handler func(w ResponseWriter, r *Request)) {
	ro.registerFunc(pattern, handler, MethodPost)
}

// Records the given pattern and handler to handle the corresponding path only on PUT method.
func (ro *Router) Put(pattern string, handler Handler) {
	ro.register(pattern, handler, MethodPut)
}

// Similar to Put method, but this method get a handler as a func
// and wrap it, to act like a Handler.
func (ro *Router) PutFunc(pattern string, handler func(w ResponseWriter, r *Request)) {
	ro.registerFunc(pattern, handler, MethodPut)
}

// Records the given pattern and handler to handle the corresponding path only on DELETE method.
func (ro *Router) Delete(pattern string, handler Handler) {
	ro.register(pattern, handler, MethodDelete)
}

// Similar to Delete method, but this method get a handler as a func
// and wrap it, to act like a Handler.
func (ro *Router) DeleteFunc(pattern string, handler func(w ResponseWriter, r *Request)) {
	ro.registerFunc(pattern, handler, MethodDelete)
}

func (ro *Router) namespace(name string) *routerNamespace {

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
		name: name,
		r:    ro,
		ns:   map[string]*routerNamespace{},
	}

	var ns map[string]*routerNamespace
	if n == nil {
		// hold router children (namespace list from this level)
		ns = ro.ns
	} else {
		// hold node children (namespace list from this level)
		ns = n.ns
		// set node parent to be parent of the new node
		nn.p = n
	}

	for k, v := range ns {
		last := strings.TrimPrefix(k, name+"/")
		if last == k {
			continue
		}
		delete(ns, k)
		v.name = last
		v.p = nn
		nn.ns[last] = v
	}

	ns[name] = nn

	return nn
}

// Creates or find an existent namespace from the router.
// The given name can be created with a single name:
//
//	"api"
//
// Or a compound name (names separated by slash):
//
//	"api/v1/media"
//
// The param will be transformed into generic param (closed brackets - {})
//
// Finally, returns the created namespace.
func (ro *Router) Namespace(name string) *namespace {
	ro.mu.Lock()
	defer ro.mu.Unlock()

	if !isValidNamespace(name) {
		panic(PanicMsgInvalidNamespace)
	}

	var params []string
	name, params = parseNamespace(name)

	return &namespace{
		n:      ro.namespace(name),
		params: params,
	}
}

// Register one or more middlewares to intercept requests.
// These middleware can be registered in the router itself,
// or in the given path (namespace).
//
// To register middleware in the router, just:
//
//	router.Use(middleware) // router.Use(middleware1, middleware2,...) for 2+ middlewares
//
// To register middleware into path:
//
//	router.Use("/path", middleware) // router.Use("/path", middleware1, middleware2,...)
func (ro *Router) Use(v any, mws ...Middleware) {

	switch got := v.(type) {
	case MiddlewareErrorHandler:
		ro.addMiddlewareErrorHandler(got)
	case string:
		ro.addMiddlewareOnPath(got, mws...)
	case Middleware:
		mws = append([]Middleware{got}, mws...)
		ro.addMiddlewareOnRouter(mws...)
	}
}

// Similar to Use method, but all the given middlewares must be a func.
//
// A common middleware must be a function with signature equal to
// func(ResponseWriter, *Request, NextMiddlewareCaller).
//
// The MiddlewareErrorHandler must be a function with the signature
// equal to func(ResponseWriter, *Request, error).
func (ro *Router) UseFunc(v any, mws ...func(ResponseWriter, *Request, NextMiddlewareCaller)) {

	switch got := v.(type) {
	case func(ResponseWriter, *Request, error):
		ro.addMiddlewareErrorHandler(MiddlewareErrorHandlerFunc(got))
	case string:
		_mws := []Middleware{}
		for i := 0; i < len(mws); i++ {
			_mws = append(_mws, Middleware(MiddlewareFunc(mws[i])))
		}
		ro.addMiddlewareOnPath(got, _mws...)
	case func(ResponseWriter, *Request, NextMiddlewareCaller):
		_mws := []Middleware{Middleware(MiddlewareFunc(got))}
		for i := 0; i < len(mws); i++ {
			_mws = append(_mws, Middleware(MiddlewareFunc(mws[i])))
		}
		ro.addMiddlewareOnRouter(_mws...)
	}
}

func (ro *Router) addMiddlewareOnRouter(mws ...Middleware) {
	ro.mu.Lock()
	defer ro.mu.Unlock()

	ro.mws = append(ro.mws, mws...)
}

func (ro *Router) addMiddlewareOnPath(path string, mws ...Middleware) {
	ro.mu.Lock()
	defer ro.mu.Unlock()

	n := ro.namespace(path)
	n.mws = append(n.mws, mws...)
}

func (ro *Router) addMiddlewareErrorHandler(meh MiddlewareErrorHandler) {
	ro.mu.Lock()
	defer ro.mu.Unlock()

	ro.meh = meh
}

type routerNamespace struct {
	name string
	r    *Router
	p    *routerNamespace // parent
	ns   map[string]*routerNamespace
	mws  []Middleware
	es   *routerEntry
	eu   *routerEntry
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

	nn := &routerNamespace{
		name: name,
		r:    na.r,
		p:    na,
		ns:   map[string]*routerNamespace{},
	}

	var ns map[string]*routerNamespace
	if n == nil {
		ns = na.ns
	} else {
		ns = n.ns
		nn.p = n
	}

	for k, v := range ns {
		last := strings.TrimPrefix(k, name+"/")
		if last == k {
			continue
		}
		delete(ns, k)
		v.name = last
		v.p = nn
		nn.ns[last] = v
	}

	ns[name] = nn // ignoring slash

	return nn
}

func (na *routerNamespace) path() string {
	var acc string
	for curr := na; curr != nil; {
		acc = "/" + curr.name + acc
		curr = curr.p
	}
	return acc
}

type namespace struct {
	n      *routerNamespace
	params []string
}

// Creates or find an existent namespace from the namespace.
// The given name can be created with a single name:
//
//	"api"
//
// Or a compound name (names separated by slash):
//
//	"api/v1/media"
//
// The param will be transformed into generic param (closed brackets - {})
//
// Finally, returns the created namespace.
func (na *namespace) Namespace(name string) *namespace {
	n := na.n
	r := n.r
	r.mu.Lock()
	defer r.mu.Unlock()

	if !isValidNamespace(name) {
		panic(PanicMsgInvalidNamespace)
	}

	name, na.params = parseNamespace(name)

	return &namespace{
		n: n.namespace(name),
	}
}

func distributeParams(pattern string, params []string) string {
	var i int
	return regexp.MustCompile(`\{\}`).ReplaceAllStringFunc(pattern, func(s string) string {
		ret := params[i]
		i++
		return ret
	})
}

func (na *namespace) register(pattern string, handler Handler, method string) {
	na.n.r.mu.Lock()
	defer na.n.r.mu.Unlock()

	if pattern != "" && !isValidPattern(pattern) {
		panic(PanicMsgInvalidPattern)
	}

	if handler == nil {
		panic(PanicMsgEmptyHandler)
	}

	name, params := parseNamespace(pattern)
	params = append(na.params, params...)

	var n *routerNamespace
	if name == "" {
		n = na.n
	} else {
		n = na.n.namespace(name)
	}

	slashed := pattern != "" && pattern[len(pattern)-1] == '/'

	var holdEntry **routerEntry
	if slashed {
		holdEntry = &n.es
	} else {
		holdEntry = &n.eu
	}

	if *holdEntry != nil {
		entry := **holdEntry
		if _, ok := entry.mh[method]; ok {
			panic(PanicMsgEndpointDuplication)
		}
		entry.mh[method] = handler
		return
	}

	if slashed {
		pattern = n.path() + "/"
	} else {
		pattern = n.path()
	}

	pattern = distributeParams(pattern, params)

	*holdEntry = &routerEntry{
		pattern: pattern,
		re:      createRegExp(pattern),
		mh: map[string]Handler{
			method: handler,
		},
	}
}

func (na *namespace) switchRegister(method string, v any, handler ...Handler) {
	switch value := v.(type) {
	case string:
		if value == "" {
			panic(PanicMsgInvalidPattern)
		}
		if len(handler) == 0 {
			panic(PanicMsgMissingHandler)
		}
		na.register(value, handler[0], method)
	case Handler:
		na.register("", value, method)
	default:
		panic("router: invalid type")
	}
}

func (na *namespace) switchRegisterFunc(method string, v any, handler ...HandlerFunc) {
	switch value := v.(type) {
	case string:
		if value == "" {
			panic(PanicMsgInvalidPattern)
		}
		if len(handler) == 0 {
			panic(PanicMsgMissingHandler)
		}
		na.register(value, handler[0], method)
	case HandlerFunc:
		na.register("", value, method)
	default:
		panic("router: invalid type")
	}
}

// Allow to register a handler able to handle to any request method that matches the pattern.
// There are 3 ways.
//
// It's possible to register the handler to the namespace path + "/", like http&#58;//site.com/nspath/;
//
//	namespace.All("/", handler)
//
// Or to the namespace path without "/" at the end, like http&#58;//site.com/nspath
//
//	namespace.All(handler)
//
// Or to a path beyond the namespace path, where the addition path is given by a pattern (accepts params too),
// like http&#58;//site.com/nspath/addition_path
//
//	namespace.All("/addition_path", handler) // namespace.All("/addition_path/{param}", handler)
func (na *namespace) All(v any, handler ...Handler) {
	na.switchRegister(MethodAll, v, handler...)
}

// Similar to All(), but this expect a func as handler
func (na *namespace) AllFunc(v any, handler ...HandlerFunc) {
	na.switchRegisterFunc(MethodAll, v, handler...)
}

// Similar to the All(), but corresponds only to GET requests
func (na *namespace) Get(v any, handler ...Handler) {
	na.switchRegister(MethodGet, v, handler...)
}

// Similar to Get(), but this expect a func as handler
func (na *namespace) GetFunc(v any, handler ...HandlerFunc) {
	na.switchRegisterFunc(MethodGet, v, handler...)
}

// Similar to the All(), but corresponds only to POST requests
func (na *namespace) Post(v any, handler ...Handler) {
	na.switchRegister(MethodPost, v, handler...)
}

// Similar to Post(), but this expect a func as handler
func (na *namespace) PostFunc(v any, handler ...HandlerFunc) {
	na.switchRegisterFunc(MethodPost, v, handler...)
}

// Similar to the All(), but corresponds only to PUT requests
func (na *namespace) Put(v any, handler ...Handler) {
	na.switchRegister(MethodPut, v, handler...)
}

// Similar to Put(), but this expect a func as handler
func (na *namespace) PutFunc(v any, handler ...HandlerFunc) {
	na.switchRegisterFunc(MethodPut, v, handler...)
}

// Similar to the All(), but corresponds only to DELETE requests
func (na *namespace) Delete(v any, handler ...Handler) {
	na.switchRegister(MethodDelete, v, handler...)
}

// Similar to Delete(), but this expect a func as handler
func (na *namespace) DeleteFunc(v any, handler ...HandlerFunc) {
	na.switchRegisterFunc(MethodDelete, v, handler...)
}

// Register one or more middlewares to intercept requests.
// These middlewares will be registered in the namespace.
func (na *namespace) Use(mw ...Middleware) {
	n := na.n
	r := n.r
	r.mu.Lock()
	defer r.mu.Unlock()

	n.mws = append(n.mws, mw...)
}

func (na *namespace) UseFunc(mw ...func(ResponseWriter, *Request, NextMiddlewareCaller)) {
	n := na.n
	r := n.r
	r.mu.Lock()
	defer r.mu.Unlock()

	_mw := []Middleware{}
	for i := 0; i < len(mw); i++ {
		_mw = append(_mw, Middleware(MiddlewareFunc(mw[i])))
	}

	n.mws = append(n.mws, _mw...)
}

package router

import (
	"bytes"
	"container/list"
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
	PanicMsgIncompatibleArgType = "router: there's a incompatible argumment type"
	PanicMsgMissingMiddleware   = "router: missing middleware"
)

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
	w.(*responseWriter).WriteHeader(http.StatusNotFound)
}

// Holds a simple request handler that replies HTTP 404 status
var NotFoundHandler = &notFoundHandler{}

type redirectHandler struct {
	url  string
	code int
}

func (rh *redirectHandler) ServeHTTP(w ResponseWriter, r *Request) {
	http.Redirect(w.(*responseWriter), r.Request, rh.url, rh.code)
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

func closer(ns *namespaceList, name string) (n *routerNamespace, path string) {
	subnames := strings.Split(name, "/")

	var acc string
	for i := 0; i < len(subnames); i++ {
		name := subnames[i]
		acc += name
		if found := ns.Find(acc); found != nil { // Exact match
			n = found
			ns = n.ns // next level
			path += acc + "/"
			acc = ""
		} else {
			found := ns.FindFunc(func(n *routerNamespace) bool {
				if strings.HasPrefix(n.name, acc) {
					return true
				}
				if strings.HasPrefix(n.name, "{}") {
					acc = "{}"
					return true
				}
				return false
			})
			if found == nil {
				break
			}
			last := strings.TrimPrefix(found.name, acc)
			for last != "" {
				last = strings.TrimPrefix(last, "/")
				if i++; i == len(subnames) {
					break
				}
				newLast := strings.TrimPrefix(last, subnames[i])
				if newLast != last {
					last = newLast
					continue
				}
				newLast = strings.TrimPrefix(last, "{}")
				if newLast != last {
					last = newLast
					continue
				}
				break
			}
			if last == "" {
				n = found
				ns = n.ns // next level
				path += found.name + "/"
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
	name = strings.Trim(name, "/")

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

type namespaceList struct {
	l *list.List
}

func newNamespaceList() *namespaceList {
	return &namespaceList{
		list.New(),
	}
}

func (nsl *namespaceList) Len() int {
	return nsl.l.Len()
}

func (nsl *namespaceList) Find(name string) *routerNamespace {
	e := nsl.l.Front()
	for e != nil {
		if e.Value.(*routerNamespace).name < name {
			e = e.Next()
			continue
		}
		break
	}
	if e == nil {
		return nil
	}
	if n := e.Value.(*routerNamespace); n.name == name {
		return n
	}
	return nil
}

func (nsl *namespaceList) FindFunc(fn func(*routerNamespace) bool) *routerNamespace {
	e := nsl.l.Front()
	for e != nil {
		if n := e.Value.(*routerNamespace); fn(n) {
			return n
		}
		e = e.Next()
	}
	return nil
}

func (nsl *namespaceList) Remove(n *routerNamespace) {
	e := nsl.l.Front()
	for e != nil {
		if e.Value.(*routerNamespace).name < n.name {
			e = e.Next()
			continue
		}
		break
	}
	if e != nil && e.Value.(*routerNamespace) == n {
		nsl.l.Remove(e)
	}
}

func (nsl *namespaceList) Add(n *routerNamespace) {
	e := nsl.l.Front()
	for e != nil {
		if e.Value.(*routerNamespace).name < n.name {
			e = e.Next()
			continue
		}
		break
	}
	if e == nil {
		nsl.l.PushBack(n)
		return
	}
	if e.Value.(*routerNamespace).name != n.name {
		nsl.l.InsertBefore(n, e)
	}
}

// Like to standard ServeMux, it's a HTTP request multiplexer.
// Have similar characteristics, however Router brings the
// possibility to handle params that can be exposed in patterns.
//
// The pattern can have params, which are added with its name
// rounded by brackets, like "/customers/{id}".
type Router struct {
	mu   sync.RWMutex
	ns   *namespaceList
	mws  []Middleware
	meh  MiddlewareErrorHandler
	e    *routerEntry // handle with "/" (the root)
	host bool
}

func NewRouter() *Router {
	return &Router{
		ns: newNamespaceList(),
	}
}

// Dispatches the request to the correspondent handler.
//
// Once the handler has been found, the request will be passed
// through the middlewares accordingly to request path.
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
	ww := &responseWriter{w}
	var errors []mwError
	if errors = ro.crossMiddlewares(p, ww, rr); len(errors) > 0 {
		err := errors[0]
		if ro.meh == nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(fmt.Sprintf("Middleware Error: %s\n%s", err.err, err.stack))
		} else {
			ro.meh.Handle(ww, rr, err.err)
		}
		return
	}
	h.ServeHTTP(ww, rr)
}

func crossMiddlewaresLayer(path []string, ns *namespaceList, mw *[]Middleware, w ResponseWriter, r *Request) chan []mwError {
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
	if fwd := ns.Find(l); fwd != nil {
		errs = append(
			errs,
			<-crossMiddlewaresLayer(path[1:], fwd.ns, &fwd.mws, w, r)...,
		)
	}
	ch := make(chan []mwError, 1)
	ch <- errs
	return ch
}

func (ro *Router) crossMiddlewares(p string, w ResponseWriter, r *Request) []mwError {
	p = strings.Trim(p, "/")

	errors := <-crossMiddlewaresLayer(strings.Split(p, "/"), ro.ns, &ro.mws, w, r)
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
	}
	if h == nil {
		return "", nil, nil
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

// Register the given pattern and handler to handle the corresponding path.
// All is a generic method correspondent
func (ro *Router) All(pattern string, handler Handler) {
	ro.register(pattern, handler, MethodAll)
}

// Similar to router's All method, but expects a function to be wrapped in a Handler.
func (ro *Router) AllFunc(pattern string, handler func(w ResponseWriter, r *Request)) {
	ro.registerFunc(pattern, HandlerFunc(handler), MethodAll)
}

// Register the given pattern and handler to handle the corresponding path only on GET method.
func (ro *Router) Get(pattern string, handler Handler) {
	ro.register(pattern, handler, MethodGet)
}

// Similar to router's Get method, but expects a function to be wrapped in a Handler.
func (ro *Router) GetFunc(pattern string, handler func(w ResponseWriter, r *Request)) {
	ro.registerFunc(pattern, handler, MethodGet)
}

// Register the given pattern and handler to handle the corresponding path only on POST method.
func (ro *Router) Post(pattern string, handler Handler) {
	ro.register(pattern, handler, MethodPost)
}

// Similar to router's Post method, but expects a function to be wrapped in a Handler.
func (ro *Router) PostFunc(pattern string, handler func(w ResponseWriter, r *Request)) {
	ro.registerFunc(pattern, handler, MethodPost)
}

// Register the given pattern and handler to handle the corresponding path only on PUT method.
func (ro *Router) Put(pattern string, handler Handler) {
	ro.register(pattern, handler, MethodPut)
}

// Similar to router's Put method, but expects a function to be wrapped in a Handler.
func (ro *Router) PutFunc(pattern string, handler func(w ResponseWriter, r *Request)) {
	ro.registerFunc(pattern, handler, MethodPut)
}

// Register the given pattern and handler to handle the corresponding path only on DELETE method.
func (ro *Router) Delete(pattern string, handler Handler) {
	ro.register(pattern, handler, MethodDelete)
}

// Similar to router's Delete method, but expects a function to be wrapped in a Handler.
func (ro *Router) DeleteFunc(pattern string, handler func(w ResponseWriter, r *Request)) {
	ro.registerFunc(pattern, handler, MethodDelete)
}

func (ro *Router) namespace(name string) *routerNamespace {

	if ro.ns == nil {
		ro.ns = newNamespaceList()
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
		ns:   newNamespaceList(),
	}

	var ns *namespaceList
	if n == nil {
		// hold router children (namespace list from this level)
		ns = ro.ns
	} else {
		// hold node children (namespace list from this level)
		ns = n.ns
		// set node parent to be parent of the new node
		nn.p = n
	}

	found := ns.FindFunc(func(n *routerNamespace) bool {
		return strings.HasPrefix(n.name, name+"/")
	})
	if found != nil {
		ns.Remove(found)
		found.name = strings.TrimPrefix(found.name, name+"/")
		found.p = nn
		nn.ns.Add(found)
	}
	ns.Add(nn)
	// for k, v := range ns {
	// 	last := strings.TrimPrefix(k, name+"/")
	// 	if last == k {
	// 		continue
	// 	}
	// 	delete(ns, k)
	// 	v.name = last
	// 	v.p = nn
	// 	nn.ns[last] = v
	// }

	// ns[name] = nn

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
// Params, which can be added as follows:
//
//	"api/v1/users/{user}"
//
// Will be transformed into generic params. In the example: api/v1/users/{}
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

// Register one or more middlewares (see Middleware interface) to intercept requests.
// These middlewares can be registered in the router itself,
// or in the given path (namespace).
//
// To register middleware(s) in the router, just:
//
//	router.Use(middleware) // router.Use(middleware1, middleware2,...) for 2+ middlewares
//
// To register middleware(s) into path:
//
//	router.Use("/path", middleware) // router.Use("/path", middleware1, middleware2,...)
//
// This method is the way to update the MiddlewareErrorHandler. Only one from this type
// is possible to exist, and is responsible to handle the error coming from other
// middleware.
// To add/update, just:
//
//	router.Use(middlewareErrorHandler)
func (ro *Router) Use(v any, mws ...Middleware) {
	ro.mu.Lock()
	defer ro.mu.Unlock()

	ro.use(v, mws...)
}

// Similar to router's Use method, but all the given middlewares must be a func.
//
// A common middleware must be a function with signature equal to
// func(ResponseWriter, *Request, NextMiddlewareCaller).
//
// The MiddlewareErrorHandler must be a function with the signature
// equal to func(ResponseWriter, *Request, error).
func (ro *Router) UseFunc(v any, mws ...func(ResponseWriter, *Request, NextMiddlewareCaller)) {
	ro.mu.Lock()
	defer ro.mu.Unlock()

	var arg1 any
	switch got := v.(type) {
	case string:
		arg1 = got
	case func(ResponseWriter, *Request, NextMiddlewareCaller):
		arg1 = MiddlewareFunc(got)
	case func(ResponseWriter, *Request, error):
		arg1 = MiddlewareErrorHandlerFunc(got)
	}

	_mws := []Middleware{}
	for i := range len(mws) {
		_mws = append(_mws, MiddlewareFunc(mws[i]))
	}
	ro.use(arg1, _mws...)
}

func (ro *Router) use(v any, mws ...Middleware) {
	switch got := v.(type) {
	case MiddlewareErrorHandler:
		ro.meh = got
	case string:
		if got == "" {
			panic(PanicMsgInvalidPattern)
		}
		if len(mws) == 0 {
			panic(PanicMsgMissingMiddleware)
		}
		n := ro.namespace(got)
		n.mws = append(n.mws, mws...)
	case Middleware:
		mws = append([]Middleware{got}, mws...)
		ro.mws = append(ro.mws, mws...)
	default:
		panic(PanicMsgIncompatibleArgType)
	}
}

type routerNamespace struct {
	name string
	r    *Router
	p    *routerNamespace // parent
	ns   *namespaceList
	mws  []Middleware
	es   *routerEntry
	eu   *routerEntry
}

func (na *routerNamespace) namespace(name string) *routerNamespace {

	if na.ns == nil {
		na.ns = newNamespaceList()
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
		ns:   newNamespaceList(),
	}

	var ns *namespaceList
	if n == nil {
		ns = na.ns
	} else {
		ns = n.ns
		nn.p = n
	}

	found := ns.FindFunc(func(n *routerNamespace) bool {
		return strings.HasPrefix(n.name, name+"/")
	})
	if found != nil {
		ns.Remove(found)
		found.name = strings.TrimPrefix(found.name, name+"/")
		found.p = nn
		nn.ns.Add(found)
	}
	ns.Add(nn)

	// for k, v := range ns {
	// 	last := strings.TrimPrefix(k, name+"/")
	// 	if last == k {
	// 		continue
	// 	}
	// 	delete(ns, k)
	// 	v.name = last
	// 	v.p = nn
	// 	nn.ns[last] = v
	// }

	// ns[name] = nn // ignoring slash

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

// Creates or find an existent namespace from the router.
// The given name can be created with a single name:
//
//	"api"
//
// Or a compound name (names separated by slash):
//
//	"api/v1/media"
//
// Params, which can be added as follows:
//
//	"api/v1/users/{user}"
//
// Will be transformed into generic params. In the example: api/v1/users/{}
//
// Finally, returns the created namespace.
func (na *namespace) Namespace(name string) *namespace {
	n := na.n
	r := n.r
	r.mu.Lock()
	defer r.mu.Unlock()

	return na.namespace(name)
}

func (na *namespace) namespace(name string) *namespace {
	if !isValidNamespace(name) {
		panic(PanicMsgInvalidNamespace)
	}

	name, params := parseNamespace(name)

	return &namespace{
		n:      na.n.namespace(name),
		params: append(na.params, params...),
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
	case func(ResponseWriter, *Request):
		na.register("", HandlerFunc(value), method)
	default:
		panic(PanicMsgIncompatibleArgType)
	}
}

func func2Handler(f ...func(ResponseWriter, *Request)) []Handler {
	hds := []Handler{}
	for i := range len(f) {
		hds = append(hds, HandlerFunc(f[i]))
	}
	return hds
}

// Allow to register a handler to any request method that matches the pattern.
// There are 3 ways.
//
// It's possible to register a handler for the namespace, like http&#58;//site.com/namespace;
//
//	namespace.All(handler)
//
// Or to the namespace + / , like http&#58;//site.com/namespace/
//
//	namespace.All("/", handler)
//
// Or to a path beyond the namespace path, where the addition path is given by a pattern,
// like http&#58;//site.com/nspath/addition_path. Params are allowed too.
//
//	namespace.All("/addition_path", handler) // namespace.All("/addition_path/{param}", handler)
func (na *namespace) All(v any, handler ...Handler) {
	na.switchRegister(MethodAll, v, handler...)
}

// Similar to namespace's All method, but expects a function to be wrapped in a Handler.
func (na *namespace) AllFunc(v any, handler ...func(ResponseWriter, *Request)) {
	na.switchRegister(MethodAll, v, func2Handler(handler...)...)
}

// Except it only matches the GET requests, this must be used in the same way as it is for the
// namespace's All method.
func (na *namespace) Get(v any, handler ...Handler) {
	na.switchRegister(MethodGet, v, handler...)
}

// Similar to namespace's Get method, but expects a function to be wrapped in a Handler.
func (na *namespace) GetFunc(v any, handler ...func(ResponseWriter, *Request)) {
	na.switchRegister(MethodGet, v, func2Handler(handler...)...)
}

// Except it only matches the POST requests, this must be used in the same way as it is for the
// namespace's All method.
func (na *namespace) Post(v any, handler ...Handler) {
	na.switchRegister(MethodPost, v, handler...)
}

// Similar to namespace's Post method, but expects a function to be wrapped in a Handler.
func (na *namespace) PostFunc(v any, handler ...func(ResponseWriter, *Request)) {
	na.switchRegister(MethodPost, v, func2Handler(handler...)...)
}

// Except it only matches the PUT requests, this must be used in the same way as it is for the
// namespace's All method.
func (na *namespace) Put(v any, handler ...Handler) {
	na.switchRegister(MethodPut, v, handler...)
}

// Similar to namespace's Put method, but expects a function to be wrapped in a Handler.
func (na *namespace) PutFunc(v any, handler ...func(ResponseWriter, *Request)) {
	na.switchRegister(MethodPut, v, func2Handler(handler...)...)
}

// Except it only matches the DELETE requests, this must be used in the same way as it is for the
// namespace's All method.
func (na *namespace) Delete(v any, handler ...Handler) {
	na.switchRegister(MethodDelete, v, handler...)
}

// Similar to namespace's Delete method, but expects a function to be wrapped in a Handler.
func (na *namespace) DeleteFunc(v any, handler ...func(ResponseWriter, *Request)) {
	na.switchRegister(MethodDelete, v, func2Handler(handler...)...)
}

// Register one or more middlewares (see Middleware interface) to intercept requests.
// These middlewares can be registered in the namespace itself,
// or in the given path (advanced namespace).
//
// To register middleware(s) in the namespace, just:
//
//	namespace.Use(middleware) // namespace.Use(middleware1, ...) for 2+ middlewares
//
// To register middleware(s) into path:
//
//	namespace.Use("/path", middleware) // namespace.Use("/path", middleware1, ...) for 2= middlewares
func (na *namespace) Use(v any, mws ...Middleware) {
	n := na.n
	r := n.r
	r.mu.Lock()
	defer r.mu.Unlock()

	na.use(v, mws...)
}

// Similar to namespace's Use method, but all the given middlewares must be a func.
//
// A common middleware must be a function with signature equal to
// func(ResponseWriter, *Request, NextMiddlewareCaller).
func (na *namespace) UseFunc(v any, mws ...func(ResponseWriter, *Request, NextMiddlewareCaller)) {
	n := na.n
	r := n.r
	r.mu.Lock()
	defer r.mu.Unlock()

	var arg1 any
	switch got := v.(type) {
	case string:
		arg1 = got
	case func(ResponseWriter, *Request, NextMiddlewareCaller):
		arg1 = MiddlewareFunc(got)
	}
	_mws := []Middleware{}
	for i := range len(mws) {
		_mws = append(_mws, MiddlewareFunc(mws[i]))
	}

	na.use(arg1, _mws...)
}

func (na *namespace) use(v any, mws ...Middleware) {
	n := na.n
	switch got := v.(type) {
	case string:
		if got == "" {
			panic(PanicMsgInvalidPattern)
		}
		if len(mws) == 0 {
			panic(PanicMsgMissingMiddleware)
		}
		path := strings.TrimPrefix(got, "/")
		n = na.namespace(path).n
	case Middleware:
		mws = append([]Middleware{got}, mws...)
	default:
		panic(PanicMsgIncompatibleArgType)
	}
	n.mws = append(n.mws, mws...)
}

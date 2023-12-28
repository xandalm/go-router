package router

type RouteHandler interface {
}

type Router struct {
	m map[string]RouteHandler
}

func (r *Router) Use(pattern string, handler RouteHandler) {
	if pattern == "" {
		panic("router: invalid pattern")
	}

	if handler == nil {
		panic("router: nil handler")
	}

	if r.m == nil {
		r.m = make(map[string]RouteHandler)
	}
	r.m[pattern] = handler
}

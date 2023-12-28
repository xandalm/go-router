package router

type RouteHandler struct {
}

type Router struct {
	m map[string]RouteHandler
}

func (r *Router) Use(pattern string, handler RouteHandler) {
	if r.m == nil {
		r.m = make(map[string]RouteHandler)
	}
	r.m[pattern] = handler
}

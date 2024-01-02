package router

import "net/http"

type Request struct {
	params map[string]string
	*http.Request
}

func (r *Request) Params() map[string]string {
	if r.params == nil {
		r.params = make(map[string]string)
	}
	return r.params
}

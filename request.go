package router

import (
	"io"
	"net/http"
)

type Params map[string]string

type Request struct {
	params Params
	*http.Request
}

func (r *Request) Params() Params {
	if r.params == nil {
		r.params = make(Params)
	}
	return r.params
}

func (r *Request) ParseBodyInto(bucket *string) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}
	*bucket = string(data)
}

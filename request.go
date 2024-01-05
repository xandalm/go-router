package router

import (
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
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

func (r *Request) ParseBodyInto(v any) {

	if reflect.TypeOf(v).Kind() != reflect.Pointer {
		panic("router: a pointer must be given to the ParseBodyInto")
	}

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}

	data := string(raw)

	switch v := v.(type) {
	case *string:
		*v = data
	case *int:
		value, err := strconv.Atoi(data)
		if err != nil {
			return
		}
		*v = value
	default:
		fmt.Println(v)
	}
}

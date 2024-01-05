package router

import (
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

	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}

	data := string(raw)

	value := reflect.ValueOf(v)

	if value.Kind() != reflect.Pointer {
		panic("router: a pointer must be given to the ParseBodyInto")
	}

	e := value.Elem()

	switch e.Kind() {
	case reflect.String:
		e.SetString(data)
	case reflect.Int:
		value, err := strconv.Atoi(data)
		if err != nil {
			return
		}
		e.SetInt(int64(value))
	}
}

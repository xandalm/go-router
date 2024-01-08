package router

import (
	"encoding/json"
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

	value := reflect.ValueOf(v)

	if value.Kind() != reflect.Pointer {
		panic("router: a pointer must be given to the ParseBodyInto")
	}

	e := value.Elem()

	switch e.Kind() {
	case reflect.String:
		e.SetString(readBody(r))
	case reflect.Int:
		value, err := strconv.Atoi(readBody(r))
		if err != nil {
			return
		}
		e.SetInt(int64(value))
	case reflect.Struct:
		json.NewDecoder(r.Body).Decode(v)
	}
}

func readBody(r *Request) string {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return ""
	}
	return string(raw)
}

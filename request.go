package router

import (
	"encoding/json"
	"errors"
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

var (
	ErrPointerNeeded  = errors.New("router: a pointer must be given to the ParseBodyInto")
	ErrUnsupportedInt = errors.New("router: cannot parse request body into int")
)

func (r *Request) ParseBodyInto(v any) error {

	value := reflect.ValueOf(v)

	if value.Kind() != reflect.Pointer {
		panic(ErrPointerNeeded)
	}

	e := value.Elem()

	switch e.Kind() {
	case reflect.String:
		e.SetString(readBody(r))
	case reflect.Int:
		value, err := strconv.Atoi(readBody(r))
		if err != nil {
			return ErrUnsupportedInt
		}
		e.SetInt(int64(value))
	case reflect.Struct:
		err := json.NewDecoder(r.Body).Decode(v)
		if err != nil {
			return fmt.Errorf("router: cannot parse request body into %T", v)
		}
	default:
		return fmt.Errorf("router: %T is not supported", v)
	}
	return nil
}

func readBody(r *Request) string {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return ""
	}
	return string(raw)
}

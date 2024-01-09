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

// Request has a embedded http.Request
// in addition to its extra methods
type Request struct {
	params Params
	*http.Request
}

// Get a map that holds every recognized param from the request path
func (r *Request) Params() Params {
	if r.params == nil {
		r.params = make(Params)
	}
	return r.params
}

var (
	ErrMissingPointer   = errors.New("router: a pointer must be given to parse request body into")
	ErrUnsupportedInt   = errors.New("router: cannot parse request body into int")
	ErrUnsupportedFloat = errors.New("router: cannot parse request body into float")
	ErrNilPointer       = errors.New("router: a initialized pointer must be given to parse request body into")
)

// Try to parse request body into the v, which
// must be initialized. Actually v can be a pointer
// to int (int64), float (float64), string and struct.
// Different kinds are not supported and will cause error.
//
// Only JSON schematized request body can be parsed
// into a struct.
func (r *Request) ParseBodyInto(v any) error {

	value := getPtrValue(v)

	switch value.Kind() {
	case reflect.String:
		value.SetString(readBody(r))
	case reflect.Int:
		return r.bodyIntoInt(value)
	case reflect.Float64:
		return r.bodyIntoFloat(value)
	case reflect.Struct:
		return r.bodyIntoStruct(v)
	default:
		return fmt.Errorf("router: %T is not supported", v)
	}
	return nil
}

func getPtrValue(v any) reflect.Value {
	value := reflect.ValueOf(v)

	if value.Kind() != reflect.Pointer {
		panic(ErrMissingPointer)
	}

	if value.IsNil() {
		panic(ErrNilPointer)
	}

	return value.Elem()
}

func readBody(r *Request) string {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return ""
	}
	return string(raw)
}

func (r *Request) bodyIntoInt(v reflect.Value) error {
	value, err := strconv.Atoi(readBody(r))
	if err != nil {
		return ErrUnsupportedInt
	}
	v.SetInt(int64(value))
	return nil
}

func (r *Request) bodyIntoFloat(v reflect.Value) error {
	value, err := strconv.ParseFloat(readBody(r), 64)
	if err != nil {
		return ErrUnsupportedFloat
	}
	v.SetFloat(value)
	return nil
}

func (r *Request) bodyIntoStruct(v any) error {
	err := json.NewDecoder(r.Body).Decode(v)
	if err != nil {
		return fmt.Errorf("router: cannot parse request body into %T", v)
	}
	return nil
}

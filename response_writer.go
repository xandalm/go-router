package router

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type ResponseWriter interface {
	SetStatus(code int)
	SetHeader(key, value string)
	Write(v any) error
	WriteJSON(v any) error
}

type responseWriter struct {
	rw http.ResponseWriter
}

func (rw *responseWriter) SetStatus(code int) {
	rw.rw.WriteHeader(code)
}

func (rw *responseWriter) SetHeader(key, value string) {
	rw.rw.Header().Set(key, value)
}

func (rw *responseWriter) Write(v any) error {
	_, err := fmt.Fprint(rw.rw, v)
	return err
}

func (rw *responseWriter) WriteJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		panic(jsonEncodeError(v))
	}
	return rw.Write(string(data))
}

func jsonEncodeError(v any) string {
	return fmt.Sprintf("router: unable to represent %v as a json", v)
}

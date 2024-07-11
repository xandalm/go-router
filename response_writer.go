package router

import "net/http"

type ResponseWriter interface {
	Status(code int)
	Set(key, value string)
}

type responseWriter struct {
	rw http.ResponseWriter
}

func (rw *responseWriter) Status(code int) {
	rw.rw.WriteHeader(code)
}

func (rw responseWriter) Set(key, value string) {
	rw.rw.Header().Set(key, value)
}

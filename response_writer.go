package router

import "net/http"

type ResponseWriter interface {
	Status(code int)
}

type responseWriter struct {
	rw http.ResponseWriter
}

func (w *responseWriter) Status(code int) {
	http.ResponseWriter(w.rw).WriteHeader(code)
}

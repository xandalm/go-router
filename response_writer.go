package router

import (
	"fmt"
	"net/http"
)

var PanicMsgWritingError = "router: unexpected error writing on response writer"

type ResponseWriter interface {
	SetStatus(code int)
	SetHeader(key, value string)
	Write(v any)
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

func (rw *responseWriter) Write(v any) {
	_, err := fmt.Fprint(rw.rw, v)
	if err != nil {
		panic(PanicMsgWritingError)
	}
}

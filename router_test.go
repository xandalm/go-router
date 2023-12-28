package router

import (
	"net/http"
	"reflect"
	"testing"
)

type dummyRouteHandler struct{}

func (h *dummyRouteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

}

func TestUse(t *testing.T) {

	t.Run("panic on empty pattern", func(t *testing.T) {
		router := &Router{}

		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
		}()
		router.Use("", &dummyRouteHandler{})
	})

	t.Run("panic on nil handler", func(t *testing.T) {
		router := &Router{}

		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
		}()

		router.Use("/path", nil)
	})

	t.Run("add /user pattern", func(t *testing.T) {
		router := &Router{}

		dummyHandler := &dummyRouteHandler{}

		router.Use("/user", dummyHandler)

		handler, ok := router.m["/user"]
		if !ok {
			t.Errorf("not registered the pattern")
		}

		if !reflect.DeepEqual(dummyHandler, handler) {
			t.Errorf("got handler %+v, but want %+v", handler, dummyHandler)
		}
	})
}

func TestUseFunc(t *testing.T) {

	t.Run("panic on empty pattern", func(t *testing.T) {
		router := &Router{}

		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
		}()

		dummyHandler := func(w http.ResponseWriter, r *http.Request) {
		}

		router.UseFunc("", dummyHandler)
	})

	t.Run("panic on nil handler", func(t *testing.T) {
		router := &Router{}

		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
		}()

		router.UseFunc("/path", nil)
	})

	t.Run("add /user pattern", func(t *testing.T) {
		router := &Router{}

		dummyHandler := func(w http.ResponseWriter, r *http.Request) {
		}

		router.UseFunc("/user", dummyHandler)

		_, ok := router.m["/user"]
		if !ok {
			t.Errorf("not registered the pattern")
		}
	})
}

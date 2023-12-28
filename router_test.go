package router

import (
	"reflect"
	"testing"
)

func TestUse(t *testing.T) {
	t.Run("add /user pattern", func(t *testing.T) {
		router := &Router{}

		dummyHandler := RouteHandler{}

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

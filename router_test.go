package router

import (
	"net/http"
	"reflect"
	"testing"
)

type dummyRouteHandler struct{}

func (h *dummyRouteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

}

var dummyHandler = &dummyRouteHandler{}
var dummyHandlerFunc = func(w http.ResponseWriter, r *http.Request) {
}

func Test_Register(t *testing.T) {

	t.Run("panic on empty pattern", func(t *testing.T) {
		router := &Router{}

		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
		}()
		router.register("", dummyHandler, MethodAll)
	})

	t.Run("panic on nil handler", func(t *testing.T) {
		router := &Router{}

		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
		}()

		router.register("/path", nil, MethodAll)
	})

	t.Run("panic on re-register", func(t *testing.T) {
		router := &Router{}

		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
		}()

		router.register("/path", dummyHandler, MethodAll)
		router.register("/path", dummyHandler, MethodAll)
	})
}

func Test_RegisterFunc(t *testing.T) {

	t.Run("panic on nil handler", func(t *testing.T) {
		router := &Router{}

		defer func() {
			r := recover()
			if r == nil {
				t.Error("didn't panic")
			}
		}()

		router.registerFunc("/path", nil, MethodAll)
	})
}

func TestUse(t *testing.T) {

	t.Run("add /user pattern", func(t *testing.T) {
		router := &Router{}

		router.Use("/user", dummyHandler)

		assertRegistered(t, router, "/user")

		e := router.m["/user"]
		assertHandler(t, e.mh[MethodAll], dummyHandler)
	})
}

func TestUseFunc(t *testing.T) {

	t.Run("add /user pattern", func(t *testing.T) {
		router := &Router{}

		router.UseFunc("/user", dummyHandlerFunc)

		assertRegistered(t, router, "/user")

		e := router.m["/user"]
		assertHandlerFunc(t, e.mh[MethodAll], RouteHandlerFunc(dummyHandlerFunc))
	})
}

func TestGet(t *testing.T) {
	t.Run("add /products pattern", func(t *testing.T) {
		router := &Router{}

		router.Get("/products", dummyHandler)

		assertRegistered(t, router, "/products")

		e := router.m["/products"]
		assertHandler(t, e.mh[MethodGet], dummyHandler)
	})
}

func TestGetFunc(t *testing.T) {
	t.Run("add /products pattern", func(t *testing.T) {
		router := &Router{}

		router.GetFunc("/products", dummyHandlerFunc)

		assertRegistered(t, router, "/products")

		e := router.m["/products"]
		assertHandlerFunc(t, e.mh[MethodGet], RouteHandlerFunc(dummyHandlerFunc))
	})
}

func assertRegistered(t testing.TB, router *Router, path string) {
	t.Helper()

	if _, ok := router.m[path]; !ok {
		t.Fatal("not registered the pattern")
	}
}

func assertHandler(t testing.TB, got, want RouteHandler) {
	t.Helper()

	if got != want {
		t.Errorf("got handler %v, but want %v", got, want)
	}
}

func assertHandlerFunc(t testing.TB, got, want RouteHandler) {
	t.Helper()

	if !reflect.DeepEqual(reflect.ValueOf(got), reflect.ValueOf(want)) {
		t.Errorf("got handler %#v, but want %#v", got, want)
	}
}

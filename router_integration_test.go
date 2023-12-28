package router_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xandalm/router"
)

type MockRouterHandler struct {
	OnHandleFunc func(http.ResponseWriter, *http.Request)
}

func (h *MockRouterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.OnHandleFunc(w, r)
}

type uriTest struct {
	uri            string
	expectedStatus int
	expectedBody   string
}

type testPathCase struct {
	path    string
	handler *MockRouterHandler
	tests   []uriTest
}

func newDummyURI(path string) string {
	return "http://site.com" + path
}

func TestUseOnGETRequest(t *testing.T) {
	router := &router.Router{}

	cases := []testPathCase{
		{
			path: "/v1/users",
			handler: &MockRouterHandler{
				OnHandleFunc: func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprint(w, `[{"Name": "Alex"}, {"Name": "Andre"}]`)
				},
			},
			tests: []uriTest{
				{newDummyURI("/v1/users"), http.StatusOK, `[{"Name": "Alex"}, {"Name": "Andre"}]`},
			},
		},
		{
			path: "/users",
			handler: &MockRouterHandler{
				OnHandleFunc: func(w http.ResponseWriter, r *http.Request) {
					fmt.Fprint(w, `[{"Name": "Alex"}, {"Name": "Andre"}]`)
				},
			},
			tests: []uriTest{
				{newDummyURI("/users"), http.StatusOK, `[{"Name": "Alex"}, {"Name": "Andre"}]`},
			},
		},
	}

	for _, c := range cases {

		t.Run(fmt.Sprintf("after added %q path", c.path), func(t *testing.T) {

			router.Use(c.path, c.handler)

			for _, test := range c.tests {
				t.Run(fmt.Sprintf("GET on %q", test.uri), func(t *testing.T) {

					request, _ := http.NewRequest(http.MethodGet, test.uri, nil)
					response := httptest.NewRecorder()

					router.ServeHTTP(response, request)

					status := response.Code

					if status != test.expectedStatus {
						t.Errorf("got status %d, but want %d", status, test.expectedStatus)
					}

					body := response.Body.String()

					if body != test.expectedBody {
						t.Errorf("got body %q, but want %q", body, test.expectedBody)
					}
				})
			}
		})
	}
}

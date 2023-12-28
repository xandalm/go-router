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

func TestUseOnGET(t *testing.T) {
	router := &router.Router{}

	handler := &MockRouterHandler{
		OnHandleFunc: func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `[{"Name": "Alex"}, {"Name": "Andre"}]`)
		},
	}

	router.Use("/users", handler)

	request, _ := http.NewRequest(http.MethodGet, "http://site.com/users", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	status := response.Code
	expectedStatus := http.StatusOK

	if status != expectedStatus {
		t.Errorf("got status %d, but want %d", status, expectedStatus)
	}

	body := response.Body.String()
	expectedBody := `[{"Name": "Alex"}, {"Name": "Andre"}]`

	if body != expectedBody {
		t.Errorf("got body %q, but want %q", body, expectedBody)
	}
}

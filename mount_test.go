package mount

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func jsonBinder(r *http.Request, dst interface{}) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

type requestType struct {
	Data int
}

func (req *requestType) Validate() error {
	if req.Data < 0 {
		return errors.New("invalid data")
	}
	return nil
}

func TestHandler(t *testing.T) {
	var callSuccess, callError bool
	successBody := &bytes.Buffer{}
	errorBody := &bytes.Buffer{}
	invalidBody := &bytes.Buffer{}

	reset := func() {
		callSuccess = false
		callError = false
		successBody.Reset()
		successBody.WriteString("{\"data\": 1}")
		errorBody.Reset()
		errorBody.WriteString("{\"data\": -1}")
		invalidBody.Reset()
		invalidBody.WriteString("invalid")
	}

	m := New(Config{
		Binder: jsonBinder,
		SuccessHandler: func(w http.ResponseWriter, r *http.Request, res interface{}) {
			callSuccess = true
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			callError = true
		},
	})

	h := m.Handler(&requestType{}, func(ctx context.Context, req interface{}) (interface{}, error) {
		r, ok := req.(*requestType)
		if !ok {
			t.Fatalf("invalid request type")
		}
		if r.Data != 1 {
			t.Fatalf("invalid data")
		}
		return map[string]int{"ok": 1}, nil
	})

	r := httptest.NewRequest(http.MethodPost, "http://localhost", successBody)
	w := httptest.NewRecorder()

	reset()
	h.ServeHTTP(w, r)
	if !callSuccess {
		t.Fatalf("success not call")
	}
	if callError {
		t.Fatalf("error should not be called")
	}

	reset()
	r = httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if callSuccess {
		t.Fatalf("success should not be called")
	}
	if !callError {
		t.Fatalf("error not call")
	}

	reset()
	r = httptest.NewRequest(http.MethodPost, "http://localhost", errorBody)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if callSuccess {
		t.Fatalf("success should not be called")
	}
	if !callError {
		t.Fatalf("error not call")
	}

	reset()
	r = httptest.NewRequest(http.MethodPost, "http://localhost", invalidBody)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if callSuccess {
		t.Fatalf("success should not be called")
	}
	if !callError {
		t.Fatalf("error not call")
	}

	h = m.Handler(&requestType{}, func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, errors.New("some error")
	})
	r = httptest.NewRequest(http.MethodPost, "http://localhost", successBody)
	w = httptest.NewRecorder()
	reset()
	h.ServeHTTP(w, r)
	if callSuccess {
		t.Fatalf("success should not be called")
	}
	if !callError {
		t.Fatalf("error not call")
	}
}

func TestDefault(t *testing.T) {
	m := New(Config{})
	i := 0
	h := m.Handler(&requestType{}, func(ctx context.Context, req interface{}) (interface{}, error) {
		if i == 0 {
			i++
			return req, nil
		}
		return nil, errors.New("some error")
	})
	for i := 0; i < 2; i++ {
		body := &bytes.Buffer{}
		body.WriteString("{\"data\": 1}")
		r := httptest.NewRequest(http.MethodPost, "http://localhost", body)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
	}
}

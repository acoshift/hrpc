package hrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func jsonRequestDecoder(r *http.Request, dst interface{}) error {
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

	var r *http.Request
	var w *httptest.ResponseRecorder

	reset := func() {
		callSuccess = false
		callError = false
		successBody.Reset()
		successBody.WriteString("{\"data\": 1}")
		errorBody.Reset()
		errorBody.WriteString("{\"data\": -1}")
		invalidBody.Reset()
		invalidBody.WriteString("invalid")
		w = httptest.NewRecorder()
	}

	m := New(Config{
		RequestDecoder: jsonRequestDecoder,
		ResponseEncoder: func(w http.ResponseWriter, r *http.Request, res interface{}) {
			callSuccess = true
		},
		ErrorEncoder: func(w http.ResponseWriter, r *http.Request, err error) {
			callError = true
		},
	})

	h := m.Handler(func(ctx context.Context, req *requestType) (interface{}, error) {
		if req.Data != 1 {
			t.Fatalf("invalid data")
		}
		return map[string]int{"ok": 1}, nil
	})

	mustSuccess := func() {
		if !callSuccess {
			t.Fatalf("success not call")
		}
		if callError {
			t.Fatalf("error should not be called")
		}
	}

	mustError := func() {
		if callSuccess {
			t.Fatalf("success should not be called")
		}
		if !callError {
			t.Fatalf("error not call")
		}
	}

	mustNothing := func() {
		if callSuccess {
			t.Fatalf("success should not be called")
		}
		if callError {
			t.Fatalf("error should not be called")
		}
	}

	reset()
	r = httptest.NewRequest(http.MethodPost, "http://localhost", successBody)
	h.ServeHTTP(w, r)
	mustSuccess()

	reset()
	r = httptest.NewRequest(http.MethodGet, "http://localhost", nil)
	h.ServeHTTP(w, r)
	mustError()

	reset()
	r = httptest.NewRequest(http.MethodPost, "http://localhost", errorBody)
	h.ServeHTTP(w, r)
	mustError()

	reset()
	r = httptest.NewRequest(http.MethodPost, "http://localhost", invalidBody)
	h.ServeHTTP(w, r)
	mustError()

	h = m.Handler(func(ctx context.Context, req *requestType) (interface{}, error) {
		return nil, errors.New("some error")
	})
	r = httptest.NewRequest(http.MethodPost, "http://localhost", successBody)
	reset()
	h.ServeHTTP(w, r)
	mustError()

	h = m.Handler(func(r *http.Request, req *requestType) (interface{}, error) {
		return map[string]int{"ok": 1}, nil
	})
	r = httptest.NewRequest(http.MethodPost, "http://localhost", successBody)
	reset()
	h.ServeHTTP(w, r)
	mustSuccess()

	h = m.Handler(func(w http.ResponseWriter, r *http.Request) {})
	r = httptest.NewRequest(http.MethodPost, "http://localhost", successBody)
	reset()
	h.ServeHTTP(w, r)
	mustNothing()

	// grpc style
	h = m.Handler(func(ctx context.Context, req *requestType, opts ...interface{}) (interface{}, error) {
		return map[string]string{"ok": "1"}, nil
	})
	r = httptest.NewRequest(http.MethodPost, "http://localhost", successBody)
	reset()
	h.ServeHTTP(w, r)
	mustSuccess()
}

func TestDefault(t *testing.T) {
	m := New(Config{})
	i := 0
	h := m.Handler(func(ctx context.Context, req *requestType) (interface{}, error) {
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

func TestInvalidF(t *testing.T) {
	p := func() {
		r := recover()
		if r == nil {
			t.Fatal("should panic")
		}
	}
	m := New(Config{})
	func() {
		defer p()
		m.Handler(1)
	}()
	func() {
		defer p()
		m.Handler(func(ctx interface{}, req interface{}) (interface{}, error) {
			return nil, nil
		})
	}()
	func() {
		defer p()
		m.Handler(func(ctx context.Context, req interface{}) (interface{}, interface{}) {
			return nil, nil
		})
	}()
}

func ExampleMounter() {
	jsonHandler := func(w http.ResponseWriter, v interface{}) {
		err := json.NewEncoder(w).Encode(v)
		if err != nil {
			fmt.Fprintf(w, "encode json error; %v", err)
		}
	}

	m := New(Config{
		RequestDecoder: func(r *http.Request, dst interface{}) error {
			// RequestDecoder will called if f contains an interface{}
			return json.NewDecoder(r.Body).Decode(dst)
		},
		ResponseEncoder: func(w http.ResponseWriter, r *http.Request, res interface{}) {
			// success handler will called if f returns an interface{}
			jsonHandler(w, res)
		},
		ErrorEncoder: func(w http.ResponseWriter, r *http.Request, err error) {
			// error handler will called if f returns an error
			res := &struct {
				Error string `json:"error"`
			}{err.Error()}
			jsonHandler(w, res)
		},
	})

	http.Handle("/user.get", m.Handler(func(ctx context.Context, req *struct {
		ID string `json:"id"`
	}) (map[string]string, error) {
		return map[string]string{
			"user_id":   req.ID,
			"user_name": "User " + req.ID,
		}, nil
	}))
	// $ curl -X POST -d '{"id":"123"}' http://localhost:8080/user.get
	// {"user_id":"123","user_name":"User 123"}

	http.Handle("/upload", m.Handler(func(r *http.Request) error {
		buf := &bytes.Buffer{}
		_, err := io.Copy(buf, r.Body)
		if err != nil {
			return err
		}
		fmt.Printf("upload data: %s\n", buf.String())
		return nil
	}))
	// $ echo "test data" | curl -X POST -d "@-" http://localhost:8080/upload
	// upload data: test data

	http.ListenAndServe(":8080", nil)
}

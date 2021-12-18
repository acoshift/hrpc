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

func jsonDecoder(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

type requestType struct {
	Data int
}

func (req *requestType) Valid() error {
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

	m := Manager{
		Decoder: jsonDecoder,
		Encoder: func(w http.ResponseWriter, r *http.Request, res any) {
			callSuccess = true
		},
		ErrorEncoder: func(w http.ResponseWriter, r *http.Request, err error) {
			callError = true
		},
		Validate: true,
	}

	h := m.Handler(func(ctx context.Context, req *requestType) (any, error) {
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

	h = m.Handler(func(ctx context.Context, req *requestType) (any, error) {
		return nil, errors.New("some error")
	})
	r = httptest.NewRequest(http.MethodPost, "http://localhost", successBody)
	reset()
	h.ServeHTTP(w, r)
	mustError()

	h = m.Handler(func(r *http.Request, req *requestType) (any, error) {
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
	h = m.Handler(func(ctx context.Context, req *requestType, opts ...any) (any, error) {
		return map[string]string{"ok": "1"}, nil
	})
	r = httptest.NewRequest(http.MethodPost, "http://localhost", successBody)
	reset()
	h.ServeHTTP(w, r)
	mustSuccess()

	// non-pointer struct request and response
	h = m.Handler(func(req requestType) (res struct {
		OK string `json:"ok"`
	}, err error) {
		if req.Data != 1 {
			t.Fatalf("invalid data")
		}
		res.OK = "1"
		return
	})
	r = httptest.NewRequest(http.MethodPost, "http://localhost", successBody)
	reset()
	h.ServeHTTP(w, r)
	mustSuccess()
}

func TestDefault(t *testing.T) {
	m := Manager{}
	i := 0
	h := m.Handler(func(ctx context.Context, req *requestType) (any, error) {
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
	m := Manager{}
	func() {
		defer p()
		m.Handler(1)
	}()
	func() {
		defer p()
		m.Handler(func(ctx any, req any) (any, error) {
			return nil, nil
		})
	}()
	func() {
		defer p()
		m.Handler(func(ctx context.Context, req any) (any, any) {
			return nil, nil
		})
	}()
}

func ExampleManager() {
	m := Manager{
		Decoder: func(r *http.Request, dst any) error {
			return json.NewDecoder(r.Body).Decode(dst)
		},
		Encoder: func(w http.ResponseWriter, r *http.Request, res any) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			json.NewEncoder(w).Encode(res)
		},
		ErrorEncoder: func(w http.ResponseWriter, r *http.Request, err error) {
			res := &struct {
				Error string `json:"error"`
			}{err.Error()}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(res)
		},
		Validate: true,
	}

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

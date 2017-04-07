package mount

import (
	"net/http"
	"reflect"

	"github.com/acoshift/httperror"
)

// Mounter type
type Mounter struct {
	c Config
}

// Config is the mounter config
type Config struct {
	Binder         func(*http.Request, interface{}) error
	SuccessHandler func(w http.ResponseWriter, r *http.Request, res interface{})
	ErrorHandler   func(w http.ResponseWriter, r *http.Request, err error)
}

// Validatable interface
type Validatable interface {
	Validate() error
}

// New creates new mounter
func New(config Config) *Mounter {
	m := &Mounter{Config{
		Binder:         func(*http.Request, interface{}) error { return nil },
		SuccessHandler: func(w http.ResponseWriter, r *http.Request, res interface{}) {},
		ErrorHandler:   func(w http.ResponseWriter, r *http.Request, err error) {},
	}}
	if config.Binder != nil {
		m.c.Binder = config.Binder
	}
	if config.SuccessHandler != nil {
		m.c.SuccessHandler = config.SuccessHandler
	}
	if config.ErrorHandler != nil {
		m.c.ErrorHandler = config.ErrorHandler
	}
	return m
}

// Handler func
func (m *Mounter) Handler(f interface{}) http.Handler {
	fv := reflect.ValueOf(f)
	ft := fv.Type()
	if ft.Kind() != reflect.Func {
		panic("f must be a function")
	}
	if ft.NumIn() != 2 {
		panic("f must have 2 inputs")
	}
	if ft.NumOut() != 2 {
		panic("f must have 2 outputs")
	}
	if ft.In(0).String() != "context.Context" {
		panic("f input 0 must be context.Context")
	}
	typ := ft.In(1)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			m.c.ErrorHandler(w, r, httperror.MethodNotAllowed)
			return
		}
		req := reflect.New(typ).Interface()
		err := m.c.Binder(r, req)
		if err != nil {
			m.c.ErrorHandler(w, r, httperror.BadRequestWith(err))
			return
		}
		if req, ok := req.(Validatable); ok {
			err = req.Validate()
			if err != nil {
				m.c.ErrorHandler(w, r, httperror.BadRequestWith(err))
				return
			}
		}
		res := fv.Call([]reflect.Value{reflect.ValueOf(r.Context()), reflect.ValueOf(req)})
		if err, ok := res[1].Interface().(error); ok && err != nil {
			m.c.ErrorHandler(w, r, err)
			return
		}
		m.c.SuccessHandler(w, r, res[0].Interface())
	})
}

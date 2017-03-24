package mount

import (
	"context"
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

// Func type
type Func func(context.Context, interface{}) (interface{}, error)

// Handler func
func (m *Mounter) Handler(req interface{}, f Func) http.Handler {
	typ := reflect.Indirect(reflect.ValueOf(req)).Type()
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
		res, err := f(r.Context(), req)
		if err != nil {
			m.c.ErrorHandler(w, r, err)
			return
		}
		m.c.SuccessHandler(w, r, res)
	})
}

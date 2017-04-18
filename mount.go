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
	SuccessHandler func(http.ResponseWriter, *http.Request, interface{})
	ErrorHandler   func(http.ResponseWriter, *http.Request, error)
}

// Validatable interface
type Validatable interface {
	Validate() error
}

// New creates new mounter
func New(config Config) *Mounter {
	m := &Mounter{Config{
		Binder:         func(*http.Request, interface{}) error { return nil },
		SuccessHandler: func(http.ResponseWriter, *http.Request, interface{}) {},
		ErrorHandler:   func(http.ResponseWriter, *http.Request, error) {},
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

type mapIndex int

const (
	_                mapIndex = iota
	miContext                 // context.Context
	miRequest                 // *http.Request
	miResponseWriter          // http.ResponseWriter
	miInterface               // interface{}
	miError                   // error
)

const (
	strContext        = "context.Context"
	strRequest        = "*http.Request"
	strResponseWriter = "http.ResponseWriter"
	strError          = "error"
)

func setOrPanic(m map[mapIndex]int, k mapIndex, v int) {
	if _, exists := m[k]; exists {
		panic("mount: duplicate input type")
	}
	m[k] = v
}

// Handler func,
// f must be a function which have at least 2 inputs and 2 outputs.
// first input must be a context.
// second input can be anything which will pass to binder function.
// first output must be the result which will pass to success handler.
// second output must be an error interface which will pass to error handler if not nil.
func (m *Mounter) Handler(f interface{}) http.Handler {
	fv := reflect.ValueOf(f)
	ft := fv.Type()
	if ft.Kind() != reflect.Func {
		panic("mount: f must be a function")
	}

	// build mapIn
	numIn := ft.NumIn()
	mapIn := make(map[mapIndex]int)
	for i := 0; i < numIn; i++ {
		switch ft.In(i).String() {
		case strContext:
			setOrPanic(mapIn, miContext, i)
		case strRequest:
			setOrPanic(mapIn, miRequest, i)
		case strResponseWriter:
			setOrPanic(mapIn, miResponseWriter, i)
		default:
			setOrPanic(mapIn, miInterface, i)
		}
	}

	// build mapOut
	numOut := ft.NumOut()
	mapOut := make(map[mapIndex]int)
	for i := 0; i < numOut; i++ {
		switch ft.Out(i).String() {
		case strError:
			setOrPanic(mapOut, miError, i)
		default:
			setOrPanic(mapOut, miInterface, i)
		}
	}

	var typ reflect.Type
	if i, ok := mapIn[miInterface]; ok {
		typ = ft.In(i)
		if typ.Kind() == reflect.Ptr {
			typ = typ.Elem()
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			m.c.ErrorHandler(w, r, httperror.MethodNotAllowed)
			return
		}

		vIn := make([]reflect.Value, numIn)
		// inject context
		if i, ok := mapIn[miContext]; ok {
			vIn[i] = reflect.ValueOf(r.Context())
		}
		// inject request interface
		if i, ok := mapIn[miInterface]; ok {
			rfReq := reflect.New(typ)
			req := rfReq.Interface()
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
			vIn[i] = rfReq
		}
		// inject request
		if i, ok := mapIn[miRequest]; ok {
			vIn[i] = reflect.ValueOf(r)
		}
		// inject response writer
		if i, ok := mapIn[miResponseWriter]; ok {
			vIn[i] = reflect.ValueOf(w)
		}

		vOut := fv.Call(vIn)
		// check error
		if i, ok := mapOut[miError]; ok {
			if vErr := vOut[i]; !vErr.IsNil() {
				if err, ok := vErr.Interface().(error); ok && err != nil {
					m.c.ErrorHandler(w, r, err)
					return
				}
			}
		}
		// check response
		if i, ok := mapOut[miInterface]; ok {
			m.c.SuccessHandler(w, r, vOut[i].Interface())
		}

		// if f is not return response, it may already call from native response writer
	})
}

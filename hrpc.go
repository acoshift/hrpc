package hrpc

import (
	"net/http"
	"reflect"
)

// Manager type
type Manager struct {
	c Config
}

// Config is the hrpc config
type Config struct {
	RequestDecoder  func(*http.Request, interface{}) error
	ResponseEncoder func(http.ResponseWriter, *http.Request, interface{})
	ErrorEncoder    func(http.ResponseWriter, *http.Request, error)
	Validate        bool // set to true to validate request after decode using Validatable interface
}

// Validatable interface
type Validatable interface {
	Validate() error
}

// New creates new manager
func New(config Config) *Manager {
	m := &Manager{config}
	if config.RequestDecoder == nil {
		m.c.RequestDecoder = func(*http.Request, interface{}) error { return nil }
	}
	if config.ResponseEncoder == nil {
		m.c.ResponseEncoder = func(http.ResponseWriter, *http.Request, interface{}) {}
	}
	if config.ErrorEncoder == nil {
		m.c.ErrorEncoder = func(http.ResponseWriter, *http.Request, error) {}
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
		panic("hrpc: duplicate input type")
	}
	m[k] = v
}

// Handler func,
// f must be a function which have at least 2 inputs and 2 outputs.
// first input must be a context.
// second input can be anything which will pass to RequestDecoder function.
// first output must be the result which will pass to success handler.
// second output must be an error interface which will pass to error handler if not nil.
func (m *Manager) Handler(f interface{}) http.Handler {
	fv := reflect.ValueOf(f)
	ft := fv.Type()
	if ft.Kind() != reflect.Func {
		panic("hrpc: f must be a function")
	}

	// build mapIn
	numIn := ft.NumIn()
	mapIn := make(map[mapIndex]int)
	for i := 0; i < numIn; i++ {
		fi := ft.In(i)

		// assume this is grpc call options
		if fi.Kind() == reflect.Slice && i == numIn-1 {
			numIn--
			break
		}

		switch fi.String() {
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
		vIn := make([]reflect.Value, numIn)
		// inject context
		if i, ok := mapIn[miContext]; ok {
			vIn[i] = reflect.ValueOf(r.Context())
		}
		// inject request interface
		if i, ok := mapIn[miInterface]; ok {
			rfReq := reflect.New(typ)
			req := rfReq.Interface()
			err := m.c.RequestDecoder(r, req)
			if err != nil {
				m.c.ErrorEncoder(w, r, err)
				return
			}

			if m.c.Validate {
				if req, ok := req.(Validatable); ok {
					err = req.Validate()
					if err != nil {
						m.c.ErrorEncoder(w, r, err)
						return
					}
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
					m.c.ErrorEncoder(w, r, err)
					return
				}
			}
		}
		// check response
		if i, ok := mapOut[miInterface]; ok {
			m.c.ResponseEncoder(w, r, vOut[i].Interface())
		}

		// if f is not return response, it may already call from native response writer
	})
}

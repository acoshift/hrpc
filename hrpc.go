package hrpc

import (
	"net/http"
	"reflect"
)

// Decoder is the request decoder
type Decoder func(*http.Request, any) error

// Encoder is the response encoder
type Encoder func(http.ResponseWriter, *http.Request, any)

// ErrorEncoder is the error response encoder
type ErrorEncoder func(http.ResponseWriter, *http.Request, error)

// Manager is the hrpc manager
type Manager struct {
	Decoder      Decoder
	Encoder      Encoder
	ErrorEncoder ErrorEncoder
	Validate     bool // set to true to validate request after decode using Validatable interface
}

func (m *Manager) decoder() Decoder {
	if m.Decoder == nil {
		return func(*http.Request, any) error { return nil }
	}
	return m.Decoder
}

func (m *Manager) encoder() Encoder {
	if m.Encoder == nil {
		return func(http.ResponseWriter, *http.Request, any) {}
	}
	return m.Encoder
}

func (m *Manager) errorEncoder() ErrorEncoder {
	if m.ErrorEncoder == nil {
		return func(http.ResponseWriter, *http.Request, error) {}
	}
	return m.ErrorEncoder
}

// Validatable interface
type Validatable interface {
	Valid() error
}

type mapIndex int

const (
	_                mapIndex = iota
	miContext                 // context.Context
	miRequest                 // *http.Request
	miResponseWriter          // http.ResponseWriter
	miAny                     // any
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
func (m *Manager) Handler(f any) http.Handler {
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
			setOrPanic(mapIn, miAny, i)
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
			setOrPanic(mapOut, miAny, i)
		}
	}

	var (
		infType reflect.Type
		infPtr  bool
	)
	if i, ok := mapIn[miAny]; ok {
		infType = ft.In(i)
		if infType.Kind() == reflect.Ptr {
			infType = infType.Elem()
			infPtr = true
		}
	}

	encoder := m.encoder()
	decoder := m.decoder()
	errorEncoder := m.errorEncoder()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vIn := make([]reflect.Value, numIn)
		// inject context
		if i, ok := mapIn[miContext]; ok {
			vIn[i] = reflect.ValueOf(r.Context())
		}
		// inject request interface
		if i, ok := mapIn[miAny]; ok {
			rfReq := reflect.New(infType)
			req := rfReq.Interface()
			err := decoder(r, req)
			if err != nil {
				errorEncoder(w, r, err)
				return
			}

			if m.Validate {
				if req, ok := req.(Validatable); ok {
					err = req.Valid()
					if err != nil {
						errorEncoder(w, r, err)
						return
					}
				}
			}
			if infPtr {
				vIn[i] = rfReq
			} else {
				vIn[i] = rfReq.Elem()
			}
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
					errorEncoder(w, r, err)
					return
				}
			}
		}
		// check response
		if i, ok := mapOut[miAny]; ok {
			encoder(w, r, vOut[i].Interface())
		}

		// if f is not return response, it may already call from native response writer
	})
}

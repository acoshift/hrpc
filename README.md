# hrpc

[![Build Status](https://travis-ci.org/acoshift/hrpc.svg?branch=master)](https://travis-ci.org/acoshift/hrpc)
[![Coverage Status](https://coveralls.io/repos/github/acoshift/hrpc/badge.svg?branch=master)](https://coveralls.io/github/acoshift/hrpc?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/acoshift/hrpc)](https://goreportcard.com/report/github.com/acoshift/hrpc)
[![GoDoc](https://godoc.org/github.com/acoshift/hrpc?status.svg)](https://godoc.org/github.com/acoshift/hrpc)

Convert RPC style function into http.Handler

## Support input types

- context.Context
- *http.Request
- http.ResponseWriter
- any

## Support output types

- any
- error

## Usage

```go
import "github.com/acoshift/hrpc/v3"
```

### Create new hrpc Manager

```go
m := hrpc.Manager{
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
```

### RPC style function

```go
type UserRequest struct {
	ID int `json:"id"`
}

func (req *UserRequest) Valid() error {
	// Valid will be called when decode, if set validate to true
	if req.ID <= 0 {
		return fmt.Errorf("invalid id")
	}
	return nil
}

type UserResponse struct {
	ID int `json:"id"`
	Username string `json:"username"`
}

http.Handle("/user.get", m.Handler(func(ctx context.Context, req *UserRequest) (*UserResponse, error) {
	return &UserResponse{ID: 1, Username: "acoshift"}, nil
}))

// or use non-ptr struct
http.Handle("/user.get2", m.Handler(func(ctx context.Context, req UserRequest) (res UserResponse, err error) {
	res.ID = 1
	res.Username = "acoshift"
	return
}))
```

### gRPC service client (generated from .proto)

```go
m.Handler(func(ctx context.Context, in *UserRequest, opts ...grpc.CallOption) (*UserResponse, error) {
	return nil, errors.New("not implemented")
})
```

### Mixed types

```go
m.Handler(func(r *http.Request) error {
	buf := &bytes.Buffer{}
	_, err := io.Copy(buf, r.Body)
	if err != nil {
		return err
	}
	fmt.Printf("upload data: %s\n", buf.String())
	return nil
})

m.Handler(func(w http.ResponseWriter, r *http.Request) {})

m.Handler(func(ctx context.Context, r *http.Request, w http.ResponseWriter) {})
```

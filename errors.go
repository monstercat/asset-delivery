package asset_delivery

import (
	"errors"
	"fmt"
	"log"
	"net/http"
)

var (
	ErrFileNotHandled = errors.New("file type not handled")
	ErrInvalidBounds  = errors.New("invalid image bounds")
)

type RootError interface {
	Root() error
}

type HTTPError interface {
	Status() int
	Error() string
}

type SystemError struct {
	RootError error
	Detail string
}

func (err *SystemError) Status() int {
	return http.StatusInternalServerError
}

func (err *SystemError) Error() string {
	return err.Detail
}

func (err *SystemError) Root() error {
	return err.RootError
}

type ParamError struct {
	Param string
	Detail string
	RootError error
}

func (err *ParamError) Status() int {
	return http.StatusBadRequest
}

func (err *ParamError) Error() string {
	return fmt.Sprintf("Bad parameter provided '%s'. %s.", err.Param, err.Detail)
}

func (err *ParamError) Root() error {
	return err.RootError
}

func WriteError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if v, ok := err.(HTTPError); ok {
		status = v.Status()
	}
	w.WriteHeader(status)
	w.Write([]byte(err.Error()))

	if v, ok := err.(RootError); ok && v.Root() != nil {
		log.Println(v.Root())
	}
}
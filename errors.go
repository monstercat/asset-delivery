package main

import (
	"fmt"
	"net/http"
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
	return err.Detail + ". " + err.RootError.Error()
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
	return fmt.Sprintf("Bad parameter provided '%s'. %s. %s", err.Param, err.Detail, err.RootError.Error())
}

func (err *ParamError) Root() error {
	return err.RootError
}
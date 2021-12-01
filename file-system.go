package main

import (
	"io"
)

type FileSystem interface {
	FromVolume(string) FileSystem
	ObjectURL(string) string
	Exists(string) (bool, error)
	ReadCloser(string) (io.ReadCloser, error)
	Write(string, io.Reader) error
	Delete(string) error
}

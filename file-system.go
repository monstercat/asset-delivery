package asset_delivery

import (
	"errors"
	"io"
	"time"
)

var ErrNoFile = errors.New("no file")

type FileSystem interface {
	FromVolume(string) FileSystem
	ObjectURL(string) string
	Info(string) (FileInfo, error)
	ReadCloser(string) (io.ReadCloser, error)
	Write(string, io.Reader, FileInfoWrite) error
	Delete(string) error
}

type FileInfoRead interface {
	Created() time.Time
}

type FileInfoWrite interface {
	CacheControl() string
}

type FileInfo interface {
	FileInfoWrite
	FileInfoRead
}


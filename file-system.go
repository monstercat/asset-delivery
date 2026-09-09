package asset_delivery

import (
	"errors"
	"io"
	"time"

	"github.com/marcw/cachecontrol"
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

// IsExpired reports whether info is past its Cache-Control max-age. An
// object with no max-age never expires (returns false).
func IsExpired(info FileInfo) bool {
	control := cachecontrol.Parse(info.CacheControl())
	if control.MaxAge() <= 0 {
		return false
	}
	return time.Now().After(info.Created().Add(control.MaxAge()))
}

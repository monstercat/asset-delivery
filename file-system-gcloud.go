package main

import (
	"context"
	"fmt"
	"io"
	"path"
	"time"

	"cloud.google.com/go/storage"
)

type GCloudFileInfo struct {
	attributes storage.ReaderObjectAttrs
}

func (i *GCloudFileInfo) CacheControl() string {
	return i.attributes.CacheControl
}

func (i *GCloudFileInfo) Created() time.Time {
	return i.attributes.LastModified
}

type GCloudFileSystem struct {
	Client *storage.Client
	Secure bool
	Bucket string
}

func (fs *GCloudFileSystem) FromVolume(name string) FileSystem {
	return &GCloudFileSystem{
		Client: fs.Client,
		Secure: fs.Secure,
		Bucket: name,
	}
}

func (fs *GCloudFileSystem) ObjectURL(filename string) string {
	var protocol string
	if fs.Secure {
		protocol = "https"
	} else {
		protocol = "http"
	}
	return fmt.Sprintf("%s://%s", protocol, path.Join("storage.googleapis.com", fs.Bucket, filename))
}

func (fs *GCloudFileSystem) Info(filename string) (FileInfo, error) {
	handle := fs.Client.Bucket(fs.Bucket).Object(filename)
	r, err := handle.NewReader(context.Background())
	if err == storage.ErrObjectNotExist {
		return nil, ErrNoFile
	}
	if err != nil {
		return nil, err
	}
	return &GCloudFileInfo{r.Attrs}, nil
}

func (fs *GCloudFileSystem) ReadCloser(filename string) (io.ReadCloser, error) {
	handle := fs.Client.Bucket(fs.Bucket).Object(filename)
	r, err := handle.NewReader(context.Background())
	return r, err
}

func (fs *GCloudFileSystem) Write(filename string, r io.Reader, info FileInfoWrite) error {
	handle := fs.Client.Bucket(fs.Bucket).Object(filename)
	w := handle.NewWriter(context.Background())
	w.CacheControl = info.CacheControl()
	defer w.Close()
	_, err := io.Copy(w, r)
	return err
}

func (fs *GCloudFileSystem) Delete(filename string) error {
	handle := fs.Client.Bucket(fs.Bucket).Object(filename)
	return handle.Delete(context.Background())
}

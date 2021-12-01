package main

import (
	"context"
	"fmt"
	"io"
	"path"

	"cloud.google.com/go/storage"
)

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

func (fs *GCloudFileSystem) Exists(filename string) (bool, error) {
	handle := fs.Client.Bucket(fs.Bucket).Object(filename)
	r, err := handle.NewReader(context.Background())
	if err == storage.ErrObjectNotExist {
		return false, nil
	} else if err != nil {
		return false, err
	}
	r.Close()
	return true, nil
}

func (fs *GCloudFileSystem) ReadCloser(filename string) (io.ReadCloser, error) {
	handle := fs.Client.Bucket(fs.Bucket).Object(filename)
	r, err := handle.NewReader(context.Background())
	return r, err
}

func (fs *GCloudFileSystem) Write(filename string, r io.Reader) error {
	fmt.Println("IM WRITING")
	handle := fs.Client.Bucket(fs.Bucket).Object(filename)
	w := handle.NewWriter(context.Background())
	defer w.Close()
	_, err := io.Copy(w, r)
	return err
}

func (fs *GCloudFileSystem) Delete(filename string) error {
	handle := fs.Client.Bucket(fs.Bucket).Object(filename)
	return handle.Delete(context.Background())
}

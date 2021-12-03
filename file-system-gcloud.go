package asset_delivery

import (
	"context"
	"io"
	"log"
	"path"
	"strings"
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
	Host   string
	Bucket string

	currBucket   string
	bucketHandle *storage.BucketHandle
}

func (fs *GCloudFileSystem) GetBucket(bucket string) *storage.BucketHandle {
	if bucket == "" {
		log.Print("Bucket name missing")
		return nil
	}
	if bucket == fs.currBucket {
		return fs.bucketHandle
	}
	fs.currBucket = bucket

	bucketHandle := fs.Client.Bucket(fs.Bucket)
	_, err := bucketHandle.Attrs(context.Background())
	if err != nil {
		log.Printf("Bucket %s does not exist.", bucket)
		return nil
	}
	fs.bucketHandle = bucketHandle
	return bucketHandle
}

func (fs *GCloudFileSystem) FromVolume(name string) FileSystem {
	return &GCloudFileSystem{
		Client: fs.Client,
		Host:   fs.Host,
		Bucket: name,
	}
}

func (fs *GCloudFileSystem) ObjectURL(filename string) string {
	host := strings.TrimSpace(fs.Host)
	if host == "" {
		host = "https://storage.googleapis.com/" + fs.Bucket
	}
	return host + path.Join("/", filename)
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
	bucket := fs.GetBucket(fs.Bucket)
	if bucket == nil {
		return ErrNoFile
	}
	handle := bucket.Object(filename)
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

package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

const MaxImageDimension = 4096

var ErrFileNotHandled = errors.New("file type not handled")
var ErrInvalidBounds = errors.New("invalid image bounds")

func main() {
	var address, credsFilename string
	flag.StringVar(&address, "address", "0.0.0.0:80", "The binding address for the application.")
	flag.StringVar(&credsFilename, "credentials", "/secrets/google.json", "The location of the Google JWT file.")
	flag.Parse()

	fs, err := NewGCloudFileSystem(credsFilename)
	if err != nil {
		log.Fatalf("Failed to create file system: %s", err.Error())
	}

	server := &Server{
		FS: fs,
	}
	err = http.ListenAndServe(address, server)
	if err != nil {
		log.Fatalf("Failed to start listening on %s: %s", address, err.Error())
	}
}

func NewGCloudFileSystem(filename string) (*GCloudFileSystem, error) {
	opts := option.WithCredentialsFile(filename)
	client, err := storage.NewClient(context.Background(), opts)
	if err != nil {
		return nil, err
	}
	return &GCloudFileSystem{
		Client: client,
		Bucket: os.Getenv("BUCKET"),
		Host:   os.Getenv("HOST"),
	}, nil
}

package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"

	. "github.com/monstercat/asset-delivery"
)

// Resize does the actual resizing operation after receiving a request from PubSub.
func main() {
	var address, credsFilename string
	flag.StringVar(&address, "address", "0.0.0.0:80", "The binding address for the application.")
	flag.StringVar(&credsFilename, "credentials", "/secrets/google.json", "The location of the Google JWT file.")
	flag.Parse()

	fs, err := NewGCloudFileSystem(credsFilename)
	if err != nil {
		log.Fatalf("Failed to create file system: %s", err.Error())
	}

	// Currently, this command creates an HTTP server that expects a PubSub request as the body of the message.
	// TODO: by flag, decide to use an HTTP server, or a subscription directly on pubsub.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var m PubSubMessage
		body, err := io.ReadAll(r.Body)
		if err != nil {
			WriteError(w, &ParamError{Param: "body", Detail: "body is missing", RootError: err})
			return
		}
		if err := json.Unmarshal(body, &m); err != nil {
			WriteError(w, &ParamError{Param: "body", Detail: "could not unmarshal body", RootError: err})
			return
		}

		// Calculate hashSum
		m.Message.Data.PopulateHash()
		if err := Resize(fs, m.Message.Data); err != nil {
			WriteError(w, err)
			return
		}

		// Return success
		w.WriteHeader(http.StatusOK)
	})

	if err := http.ListenAndServe(address, nil); err != nil {
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

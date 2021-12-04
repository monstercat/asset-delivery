package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/storage"
	"github.com/monstercat/golib/logger"
	"google.golang.org/api/option"

	. "github.com/monstercat/asset-delivery"
)

func main() {
	var address, credsFilename, allowedHosts, projectId string
	flag.StringVar(&address, "address", "0.0.0.0:80", "The binding address for the application.")
	flag.StringVar(&credsFilename, "credentials", "/secrets/google.json", "The location of the Google JWT file.")
	flag.StringVar(&allowedHosts, "allow", "", "A comma separated list of domain hosts. An empty value allows any.")
	flag.StringVar(&projectId, "project-id", "", "Project ID")
	flag.Parse()

	fs, err := NewGCloudFileSystem(credsFilename)
	if err != nil {
		log.Fatalf("Failed to create file system: %s", err.Error())
	}

	log.Print("Project ID: ", projectId)

	pb, err := NewGCloudPubSub(credsFilename, projectId)
	if err != nil {
		log.Fatalf("Failed to create connection to pubsub: %s", err.Error())
	}
	defer pb.Close()

	cloudLogger, err := NewGCloudLogger(credsFilename, projectId, "asset-delivery")
	if err != nil {
		log.Fatalf("Failed to create connection to logger: %s", err.Error())
	}

	server := &Server{
		Logger:         cloudLogger,
		FS:             fs,
		PB:             pb,
		PermittedHosts: strings.Split(allowedHosts, ","),
		Prefix:         "resized",
	}
	err = http.ListenAndServe(address, server)
	if err != nil {
		log.Fatalf("Failed to start listening on %s: %s", address, err.Error())
	}
}

func NewGCloudLogger(filename, project, name string) (*logger.Google, error) {
	opts := option.WithCredentialsFile(filename)
	client, err := logging.NewClient(context.Background(), project, opts)
	if err != nil {
		return nil, err
	}
	return logger.NewGoogle(client, name), nil
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

func NewGCloudPubSub(filename, projectId string) (*GCloudPubSub, error) {
	opts := option.WithCredentialsFile(filename)
	client, err := pubsub.NewClient(
		context.Background(),
		projectId,
		opts,
	)
	if err != nil {
		return nil, err
	}
	return &GCloudPubSub{
		Client: client,
	}, nil
}

package main

import (
	"flag"
	"log"
	"net/http"
	"strings"

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

	opts := option.WithCredentialsFile(credsFilename)

	fs, err := NewGCloudFileSystem(opts)
	if err != nil {
		log.Fatalf("Failed to create file system: %s", err.Error())
	}

	log.Print("Project ID: ", projectId)

	pb, err := NewGCloudPubSub(projectId, opts)
	if err != nil {
		log.Fatalf("Failed to create connection to pubsub: %s", err.Error())
	}
	defer pb.Close()

	cloudClient, cloudLogger, err := NewGCloudLogger(projectId, "asset-delivery", opts)
	if err != nil {
		log.Fatalf("Failed to create connection to logger: %s", err.Error())
	}
	defer cloudClient.Close()

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
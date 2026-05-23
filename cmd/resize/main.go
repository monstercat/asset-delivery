package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"google.golang.org/api/option"

	. "github.com/monstercat/asset-delivery"
)

func main() {
	var address, credsFilename, projectId string
	flag.StringVar(&address, "address", "", "The binding address. Defaults to 0.0.0.0:$PORT (Cloud Run sets PORT, default 8080).")
	flag.StringVar(&credsFilename, "credentials", "", "Path to a Google JWT credentials file. Empty uses ADC.")
	flag.StringVar(&projectId, "project-id", "", "GCP project ID (used for Cloud Logging).")
	flag.Parse()

	if address == "" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		address = "0.0.0.0:" + port
	}

	var clientOpts []option.ClientOption
	if credsFilename != "" {
		clientOpts = append(clientOpts, option.WithCredentialsFile(credsFilename))
	}

	log.Print("Project ID: ", projectId)

	fs, err := NewGCloudFileSystem(clientOpts...)
	if err != nil {
		log.Fatalf("Failed to create file system: %s", err.Error())
	}

	cloudClient, cloudLogger, err := NewGCloudLogger(projectId, "asset-resize", clientOpts...)
	if err != nil {
		log.Fatalf("Failed to create connection to logger: %s", err.Error())
	}
	defer cloudClient.Close()

	server := &Server{
		Logger: cloudLogger,
		FS:     fs,
	}

	log.Printf("Listening on %s", address)
	if err := http.ListenAndServe(address, server); err != nil {
		log.Fatalf("Failed to start listening on %s: %s", address, err.Error())
	}
}

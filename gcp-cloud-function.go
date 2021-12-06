package asset_delivery

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/monstercat/golib/logger"
)

// This file contains functionality to be used by GCP Cloud Functions. This file *must* be in the most top-level directory
// of the repo as that is where GCP can call functions from.

var (
	// GCP_PROJECT defined for the Go runtime
	// @see https://cloud.google.com/functions/docs/configuring/env-var
	gcpProject = os.Getenv("GCP_PROJECT")

)

// PubSubMessage is the payload of a Pub/Sub event. Data is strictly ResizeOptions.
// See the documentation for more details:
// https://cloud.google.com/pubsub/docs/reference/rest/v1/PubsubMessage
type PubSubMessage struct {
	Message struct {
		Data []byte `json:"data,omitempty"`
	} `json:"message"`
}

// GCF_Resize is meant to run on Google Cloud Functions
func GCF_Resize(ctx context.Context, m PubSubMessage) {
	log.Print("Project ID: ", gcpProject)

	var data ResizeOptions
	if err := json.Unmarshal(m.Message.Data, &data); err != nil {
		log.Fatalf("Could not unmarshal message. %s" ,err)
	}

	// Assumption is that creds are properly defined by default through GCP
	fs, err := NewGCloudFileSystem()
	if err != nil {
		log.Fatalf("Failed to create file system: %s", err.Error())
	}

	// Assumption is that creds are properly defined by default through GCP
	cloudClient, cloudLogger, err := NewGCloudLogger(gcpProject, "asset-delivery")
	if err != nil {
		log.Fatalf("Failed to create connection to logger: %s", err.Error())
	}
	defer cloudClient.Close()

	l := &logger.Contextual{
		Logger:  cloudLogger,
		Context: data,
	}

	// Populate the hash of the ResizeOpts
	data.PopulateHash()

	// Resize
	l.Log(logger.SeverityInfo, "Received request to resize")
	if err := Resize(fs, data); err != nil {
		if v, ok := err.(RootError); ok && v.Root() != nil {
			l.Log(logger.SeverityError, "Could not resize image: "+err.Error()+"; "+v.Root().Error())
		} else {
			l.Log(logger.SeverityError, "Could not resize image: "+err.Error())
		}
	}
}

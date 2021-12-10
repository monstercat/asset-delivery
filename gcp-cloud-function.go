package asset_delivery

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/monstercat/golib/logger"
)

// This file contains functionality to be used by GCP Cloud Functions. This file *must* be in the most top-level directory
// of the repo as that is where GCP can call functions from.

var (
	// GCP_PROJECT defined for the Go runtime
	// @see https://cloud.google.com/functions/docs/configuring/env-var
	gcpProject = os.Getenv("PROJECTID")

	defaultCacheControl = os.Getenv("DEFAULT_CACHE_CONTROL")
)

// PubSubMessage is the payload of a Pub/Sub event. Data is strictly ResizeOptions.
// See the documentation for more details:
// https://cloud.google.com/pubsub/docs/reference/rest/v1/PubsubMessage
type PubSubMessage struct {
	Data []byte `json:"data,omitempty"`
}

// GcfResize is meant to run on Google Cloud Functions
func GcfResize(ctx context.Context, m PubSubMessage) error {
	log.Print("Project ID: ", gcpProject)
	log.Printf("Request: %s", m.Data)

	// Assumption is that creds are properly defined by default through GCP
	cloudClient, cloudLogger, err := NewGCloudLogger(gcpProject, "asset-resize")
	if err != nil {
		log.Printf("Failed to create connection to logger: %s", err.Error())
		return err
	}
	defer cloudClient.Close()

	var data ResizeOptions
	if err := json.Unmarshal(m.Data, &data); err != nil {
		cloudLogger.Log(logger.SeverityError, fmt.Sprintf("Could not unmarshal message. %s", err))
		return err
	}

	// TODO: find a way to determine that an existing resize for this specific set of parameters has not already
	//  been started
	
	// Assumption is that creds are properly defined by default through GCP
	fs, err := NewGCloudFileSystem()
	if err != nil {
		cloudLogger.Log(logger.SeverityError, fmt.Sprintf("Failed to create file system: %s", err.Error()))
		return err
	}

	// Populate the hash of the ResizeOpts
	data.PopulateHash()
	log.Printf("Hash: %s", data.HashSum)

	l := &logger.Contextual{
		Logger:  cloudLogger,
		Context: data,
	}

	// Resize
	if err := Resize(fs, data); err != nil {
		if v, ok := err.(RootError); ok && v.Root() != nil {
			l.Log(logger.SeverityError, "Could not resize image: "+err.Error()+"; "+v.Root().Error())
		} else {
			log.Print("Could not resize image: "+err.Error())
			l.Log(logger.SeverityError, "Could not resize image: "+err.Error())
		}
		return err
	}

	return nil
}

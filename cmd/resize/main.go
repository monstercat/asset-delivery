package resize

import (
	"context"
	"log"
	"os"

	"github.com/monstercat/golib/logger"

	. "github.com/monstercat/asset-delivery"
)

var (
	// GCP_PROJECT defined for the Go runtime
	// @see https://cloud.google.com/functions/docs/configuring/env-var
	projectId = os.Getenv("GCP_PROJECT")

)

// ResizeSubscriber is meant to run on GCP Cloud Functions
func ResizeSubscriber(ctx context.Context, m PubSubMessage) {
	log.Print("Project ID: ", projectId)

	// Assumption is that creds are properly defined by default through GCP
	fs, err := NewGCloudFileSystem()
	if err != nil {
		log.Fatalf("Failed to create file system: %s", err.Error())
	}

	// Assumption is that creds are properly defined by default through GCP
	cloudClient, cloudLogger, err := NewGCloudLogger(projectId, "asset-delivery")
	if err != nil {
		log.Fatalf("Failed to create connection to logger: %s", err.Error())
	}
	defer cloudClient.Close()

	l := &logger.Contextual{
		Logger:  cloudLogger,
		Context: m.Message.Data,
	}

	// Populate the hash of the ResizeOpts
	m.Message.Data.PopulateHash()

	l.Log(logger.SeverityInfo, "Received request to resize")
	if err := Resize(fs, m.Message.Data); err != nil {
		if v, ok := err.(RootError); ok && v.Root() != nil {
			l.Log(logger.SeverityError, "Could not resize image: "+err.Error()+"; "+v.Root().Error())
		} else {
			l.Log(logger.SeverityError, "Could not resize image: "+err.Error())
		}
	}
}

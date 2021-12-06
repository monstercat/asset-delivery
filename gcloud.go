package asset_delivery

import (
	"context"
	"os"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/storage"
	"github.com/monstercat/golib/logger"
	"google.golang.org/api/option"
)

func NewGCloudLogger(project, name string, opts ...option.ClientOption) (*logger.Google, error) {
	client, err := logging.NewClient(context.Background(), project, opts...)
	if err != nil {
		return nil, err
	}
	return logger.NewGoogle(client, name), nil
}

func NewGCloudFileSystem(opts ...option.ClientOption) (*GCloudFileSystem, error) {
	client, err := storage.NewClient(context.Background(), opts...)
	if err != nil {
		return nil, err
	}
	return &GCloudFileSystem{
		Client: client,
		Bucket: os.Getenv("BUCKET"),
		Host:   os.Getenv("HOST"),
	}, nil
}

func NewGCloudPubSub(projectId string, opts ...option.ClientOption) (*GCloudPubSub, error) {
	client, err := pubsub.NewClient(
		context.Background(),
		projectId,
		opts...,
	)
	if err != nil {
		return nil, err
	}
	return &GCloudPubSub{
		Client: client,
	}, nil
}

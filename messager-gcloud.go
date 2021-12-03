package asset_delivery

import (
	"context"

	"cloud.google.com/go/pubsub"
)

type GCloudPubSub struct {
	*pubsub.Client
}

func (p *GCloudPubSub) Publish(subj string, data []byte) error {
	topic := p.Topic(subj)
	topic.PublishSettings.DelayThreshold = 0
	topic.PublishSettings.CountThreshold = 0

	topic.Publish(context.Background(), &pubsub.Message{
		Data: data,
	})
	return nil
}
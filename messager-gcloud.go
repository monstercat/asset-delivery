package asset_delivery

import (
	"context"

	"cloud.google.com/go/pubsub"
	"github.com/monstercat/golib/logger"
)

type GCloudPubSub struct {
	logger.Logger
	*pubsub.Client
}

func (p *GCloudPubSub) Publish(subj string, data []byte) error {
	topic := p.Topic(subj)
	topic.PublishSettings.DelayThreshold = 0
	topic.PublishSettings.CountThreshold = 1

	res := topic.Publish(context.Background(), &pubsub.Message{
		Data: data,
	})

	id, err := res.Get(context.Background())
	if p.Logger != nil {
		p.Log(logger.SeverityInfo, "Resize msg id: " + id)
	}
	return err
}
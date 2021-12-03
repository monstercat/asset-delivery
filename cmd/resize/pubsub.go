package main

import . "github.com/monstercat/asset-delivery"

// PubSubMessage is the payload of a Pub/Sub event. Data is strictly ResizeOptions.
// See the documentation for more details:
// https://cloud.google.com/pubsub/docs/reference/rest/v1/PubsubMessage
type PubSubMessage struct {
	Message struct {
		Data ResizeOptions `json:"data,omitempty"`
		ID   string        `json:"id"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}


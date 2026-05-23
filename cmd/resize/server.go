package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/monstercat/golib/logger"

	. "github.com/monstercat/asset-delivery"
)

// maxPushBodyBytes caps incoming Pub/Sub push bodies. Pub/Sub push
// payloads are well under 1 MiB in practice; the limit guards against
// malformed clients.
const maxPushBodyBytes = 10 << 20

// PubSubPushRequest is the envelope Pub/Sub delivers to a push endpoint.
// See https://cloud.google.com/pubsub/docs/push#receive_push.
type PubSubPushRequest struct {
	Message struct {
		Data        []byte            `json:"data"`
		MessageID   string            `json:"messageId"`
		PublishTime string            `json:"publishTime"`
		Attributes  map[string]string `json:"attributes,omitempty"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

// Server handles Pub/Sub push deliveries for the asset-delivery-resize
// topic and runs the resize against the configured FileSystem.
type Server struct {
	logger.Logger
	FS FileSystem
}

// ServeHTTP decodes the push envelope, unmarshals the embedded
// ResizeOptions, and runs the resize. HTTP status semantics drive
// Pub/Sub redelivery: 2xx acks the message, 4xx tells Pub/Sub the
// payload is bad (route to dead letter), 5xx triggers a retry.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPushBodyBytes))
	if err != nil {
		s.Log(logger.SeverityError, "Failed to read request body: "+err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var req PubSubPushRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.Log(logger.SeverityError, "Failed to unmarshal push envelope: "+err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var data ResizeOptions
	if err := json.Unmarshal(req.Message.Data, &data); err != nil {
		s.Log(logger.SeverityError, fmt.Sprintf("Could not unmarshal resize options (messageId=%s): %s", req.Message.MessageID, err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	data.PopulateHash()

	l := &logger.Contextual{
		Logger:  s.Logger,
		Context: data,
	}
	l.Log(logger.SeverityInfo, fmt.Sprintf("Resizing (messageId=%s, hash=%s, width=%d)", req.Message.MessageID, data.HashSum, data.Width))

	if err := Resize(s.FS, data); err != nil {
		status := http.StatusInternalServerError
		if v, ok := err.(HTTPError); ok {
			status = v.Status()
		}
		if v, ok := err.(RootError); ok && v.Root() != nil {
			l.Log(logger.SeverityError, "Could not resize image: "+err.Error()+"; "+v.Root().Error())
		} else {
			l.Log(logger.SeverityError, "Could not resize image: "+err.Error())
		}
		w.WriteHeader(status)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

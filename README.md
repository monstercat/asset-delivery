# Asset Delivery

Delivers resized assets (images) in response to HTTP requests. It contains
two parts:

- **Delivery server** (`asset-delivery` Cloud Run service)
- **Resize worker** (`asset-resize` Cloud Run service)

Both binaries are built from a single image (`cloud/build.Dockerfile`) and
copied into per-service runtime images. The pipeline is wired in
`cloudbuild.yaml`.

## Delivery Server

The delivery server attempts to find an existing resized file from a
storage location. If the resized file is found it returns the resized
file. Otherwise, it delivers the original file and publishes a resize
request to the `asset-delivery-resize` Pub/Sub topic.

### HTTP Request

Each request needs the following URL parameters:

- `width`
- `url`
- `encoding` (e.g., webp, jpeg, png)

For example, to request a version of `https://host/path` with `width=100`
and `encoding=webp`:

```
https://[host]?width=100&url=https://host/path&encoding=webp
```

### Environment Variables

- **BUCKET**: GCP Storage bucket name
- **HOST**: GCP Storage host (optional). Used by emulators.

### Command-Line Arguments

- **address**: Bind address. Defaults to `0.0.0.0:80`.
- **credentials**: Path to a Google JWT file.
- **allow**: Comma-separated allowed hosts for the `url` query param.
  Empty allows any.
- **project-id**: Google Project ID (for logging & pubsub).

## Resize Worker

The resize worker consumes messages from the `asset-delivery-resize`
Pub/Sub topic via a **push subscription**. Each push delivery is an
HTTP POST with the Pub/Sub envelope as the body
(see [Pub/Sub push docs][push-docs]). The worker unmarshals the embedded
`ResizeOptions`, resizes the source image, and writes the result to the
configured GCS bucket.

HTTP status drives Pub/Sub redelivery:

- `2xx` — message acked
- `4xx` — permanent failure (e.g., malformed payload); routed to dead
  letter after `maxDeliveryAttempts`
- `5xx` — transient failure; Pub/Sub retries with backoff

### Environment Variables

- **PROJECTID**: Google Project ID (used for Cloud Logging)
- **BUCKET**: GCP Storage bucket name
- **HOST**: GCP Storage host (optional)
- **DEFAULT_CACHE_CONTROL**: Fallback `Cache-Control` value when the
  upstream response carries none
- **PORT**: Bind port (Cloud Run sets this; defaults to `8080`)

### Pub/Sub Push Subscription

The push subscription on `projects/connect-1321/topics/asset-delivery-resize`
should be configured to POST to the `asset-resize` Cloud Run service URL.
For authenticated push, set the subscription's push auth service account
and grant it `roles/run.invoker` on `asset-resize`. Configure a
dead-letter topic + `maxDeliveryAttempts` to avoid hot-looping on poison
messages.

[push-docs]: https://cloud.google.com/pubsub/docs/push

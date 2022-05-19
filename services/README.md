# Services

The asset delivery system is separated into two services:

* HTTP Server&#x20;
* Google Cloud Function&#x20;

The HTTP server handles the incoming request and determines whether to redirect to the resized image or the original image. It will also send resize requests to the Google Cloud Function through Google PubSub.

The Google Cloud Function handles all resizing operations.&#x20;

# HTTP Server

The HTTP Server is primarily responsible for serving a response to the user with minimum latency. It is run on Google Cloud Run. Therefore, there is limited time after the expiration of a request that a service can continue running, thereby making inline resizing operations unfeasible while decreasing the amount of latency to the user.&#x20;

&#x20;TODO

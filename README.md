# Asset Delivery

Delivers resized assets (images) in response to HTTP requests. It contains two parts:
- Delivery server
- Resizing routine 

## Delivery Server 

The delivery server attempts to find an existing resized file from a storage location. If the resized file is found it 
returns the resized file. Otherwise, it will deliver the original file and send an instruction to create a resized
version of said file. 

### HTTP Request 

Each request needs to have the following as URL parameters: 
- width 
- location
- encoding (e.g., webp, jpeg, png)

For example, if you want a version of https://host/path with width=100 and encoding=webp, the resulting request would be
`https://[host]?width=100&url=https://host/path&encoding=webp`

### Environment Variables 

- **BUCKET**: GCP Storage bucket name
- **HOST**: GCP Storage host (optional). You can fill this in if an emulator is being used.

### Command-Line Arguments 

- **ADDRESS**: Address to bind to. Defaults to: 0.0.0.0:80
- **credentials**: The location of the Google JWT file.
- **allowedHosts**: A comma separated list of domain hosts. An empty value allows any.
- **project-id**: Google Project ID (for logging & pubsub) 

### TODO:

- Externalize the PubSub topic in an environment variable 


## Resizing 

Resizing is done through `GcfResize` function which is meant to be run as a Google Cloud Function. 

### Environment Variables

- **PROJECTID**: Google Project ID (used for logging)
- **BUCKET**: GCP Storage bucket name


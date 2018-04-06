# Asset Delivery

This project is an asset delivery service that will create static assets on the
fly when required. Currently the only supported storage option is Google Cloud
Storage.

## Query Features

### image_width

Tack on "image_width=NUMBER" to resize the desired asset to the specified size.

## Notices

You will need to put a "key.json" file in the location of execution so you can
connect to Google Cloud services. It should have permissions to the storage
APIs.

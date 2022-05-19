# Introduction

Asset delivery provides a centralized way to cache & resize images and videos. It is currently hosted on [https://cdx.monstercat.com](https://cdx.monstercat.com) and has the following options:&#x20;

<table><thead><tr><th>Parameter</th><th>Type</th><th data-type="checkbox">Required</th><th>Description</th></tr></thead><tbody><tr><td>url</td><td>string</td><td>true</td><td>URL to the original</td></tr><tr><td>force</td><td>boolean</td><td>false</td><td>Forces a cache reload for the specific width</td></tr><tr><td>encoding</td><td>string</td><td>true</td><td>Format of the resized image </td></tr><tr><td>width</td><td>number</td><td>true</td><td>Width to return</td></tr></tbody></table>



When the asset delivery mechanism receives a request, it checks to see if resizing is required. If so, it will send an instruction to resize the image, and will simply redirect the user to the original image. Otherwise, it will redirect the user to the resized image.&#x20;

An image will be resized if:&#x20;

* the `force` parameter is true&#x20;
* the resized file does not exist&#x20;
* the `cache-control` on the image has designated it to be expired.&#x20;

### Prior Implementations&#x20;

Our first implementation stored resized images at the application level for each application. While this worked, it caused duplicate work as each application needed to handle its own resizing operations. Furthermore, each route needed to be manually configured to allow for image resizing.&#x20;

Our second attempt utilized [CloudFlare Image Resizing](https://developers.cloudflare.com/images/image-resizing) but this was not financially viable.&#x20;

### TODOs

* Handle videos&#x20;
* Finish documentation&#x20;
* force should delete all resized images based on the hash&#x20;

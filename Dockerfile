FROM golang:alpine3.15
EXPOSE 80
ENV GCLOUD_BUCKET=minicat
RUN apk add build-base
WORKDIR src/github.com/monstercat/asset-delivery
ADD . .
RUN go get .
RUN go build -o delivery
CMD ./delivery
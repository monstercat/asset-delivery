FROM golang:alpine3.15
EXPOSE 80
ENV BUCKET="minicat"
ENV HOST=""
RUN apk add build-base
WORKDIR src/github.com/monstercat/asset-delivery
ADD . .
RUN go get .
RUN go build -o delivery
CMD ./delivery
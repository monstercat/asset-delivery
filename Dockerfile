FROM golang:alpine3.15
EXPOSE 80
ENV BUCKET="minicat"
ENV HOST=""
ENV ALLOW="www.monstercat.com, player.monstercat.app, cdn.monstercat.com, labelmanager.app, api.labelmanager.app, www.monstercat.dev"
RUN apk add build-base
WORKDIR src/github.com/monstercat/asset-delivery
ADD . .
RUN go get .
RUN go build -o delivery
CMD ./delivery -allow "$ALLOW"
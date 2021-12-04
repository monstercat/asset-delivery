FROM golang:alpine3.15
EXPOSE 80
ARG PROJECTID

# ARG is only for build time. We need it in ENV to pass to run command.
ENV PROJECTID=$PROJECTID

ENV BUCKET="minicat"
ENV HOST=""
ENV ALLOW="www.monstercat.com, player.monstercat.app, cdn.monstercat.com, labelmanager.app, api.labelmanager.app, www.monstercat.dev"
RUN apk add build-base
WORKDIR src/github.com/monstercat/asset-delivery
ADD . .
RUN go get .
RUN go build -o resize ./cmd/resize/
CMD ./resize -project-id "$PROJECTID"
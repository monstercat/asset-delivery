FROM golang:1.26-alpine
EXPOSE 80
ENV PROJECTID=""
ENV BUCKET="minicat"
ENV HOST=""
ENV ALLOW="www.monstercat.com, player.monstercat.app, cdn.monstercat.com, labelmanager.app, api.labelmanager.app, www.monstercat.dev"
RUN apk add --no-cache build-base git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /usr/local/bin/delivery ./cmd/delivery/
CMD ["sh", "-c", "exec delivery -allow \"$ALLOW\" -project-id \"$PROJECTID\""]

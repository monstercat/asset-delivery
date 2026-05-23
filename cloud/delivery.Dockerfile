FROM alpine:3.22
EXPOSE 80

ENV PROJECTID=""
ENV BUCKET="minicat"
ENV HOST=""
ENV ALLOW="www.monstercat.com, player.monstercat.app, cdn.monstercat.com, labelmanager.app, api.labelmanager.app, www.monstercat.dev"

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
ADD /binaries/delivery .

CMD ["sh", "-c", "exec ./delivery -allow \"$ALLOW\" -project-id \"$PROJECTID\""]

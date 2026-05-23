FROM alpine:3.22
EXPOSE 8080

ENV PROJECTID=""
ENV BUCKET="minicat"
ENV HOST=""
ENV DEFAULT_CACHE_CONTROL=""

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
ADD /binaries/resize .

CMD ["sh", "-c", "exec ./resize -project-id \"$PROJECTID\""]

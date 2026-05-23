FROM golang:1.26-alpine

ENV CGO_ENABLED=1

RUN apk add --no-cache build-base git

WORKDIR /src

# Cache go module downloads independently of source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN mkdir -p /src/bin \
    && go build -o /src/bin/delivery ./cmd/delivery/ \
    && go build -o /src/bin/resize   ./cmd/resize/

# Copy compiled binaries into the shared workspace bind-mounted by the
# next cloudbuild step. Subsequent per-service Dockerfiles ADD them
# from /binaries/<name>.
CMD mkdir -p /workspace/binaries \
    && cp /src/bin/delivery /workspace/binaries/delivery \
    && cp /src/bin/resize   /workspace/binaries/resize

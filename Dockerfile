# Builds any of the repository's binaries. Pass TARGET to choose:
#   docker build --build-arg TARGET=./adapters/fediverse .
ARG GO_VERSION=1.24

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

# Dependencies first so a source-only change does not re-download them.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGET=./gateway/cmd/gateway
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ${TARGET}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]

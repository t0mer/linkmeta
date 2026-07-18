FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=docker
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
    -o /app/linkmeta ./cmd/linkmeta

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/linkmeta /linkmeta
EXPOSE 8080
ENTRYPOINT ["/linkmeta"]

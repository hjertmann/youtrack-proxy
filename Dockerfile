FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Cross-compile rather than emulate: the binary is pure Go (CGO_ENABLED=0), so
# the build always runs natively on BUILDPLATFORM and just retargets the output.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /app/server .

FROM scratch
LABEL org.opencontainers.image.source="https://github.com/hjertmann/youtrack-proxy"
LABEL org.opencontainers.image.description="HTTP proxy presenting a Jira REST API surface backed by YouTrack, for Apache DevLake ingestion."
LABEL org.opencontainers.image.licenses="MIT"
# scratch has no trust store, so HTTPS to YouTrack fails with
# "x509: certificate signed by unknown authority" without this.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /app/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]

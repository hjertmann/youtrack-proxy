# Pinned to an immutable multi-arch manifest digest (golang:1.25-alpine,
# 1.25.14-alpine3.24). Renovate/Dependabot can bump the digest+comment together.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59 AS build
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

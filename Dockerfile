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
COPY --from=build /app/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]

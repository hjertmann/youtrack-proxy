FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server .

FROM scratch
LABEL org.opencontainers.image.source="https://github.com/hjertmann/youtrack-proxy"
LABEL org.opencontainers.image.description="HTTP proxy presenting a Jira REST API surface backed by YouTrack, for Apache DevLake ingestion."
LABEL org.opencontainers.image.licenses="MIT"
COPY --from=build /app/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]

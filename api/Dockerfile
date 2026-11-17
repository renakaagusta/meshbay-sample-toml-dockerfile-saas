# Build context is the repository root (Meshbay always builds from the repo
# root), so every COPY path is prefixed with api/.
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY api/go.mod api/go.sum ./
RUN go mod download
COPY api/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM alpine:3.20
RUN adduser -D -H sampleuser
COPY --from=build /out/server /app/server
COPY --from=build /out/migrate /app/migrate
USER sampleuser
ENV PORT=8080
EXPOSE 8080
CMD ["/app/server"]

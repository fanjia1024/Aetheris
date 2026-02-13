# Multi-stage build for API and Worker
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/api ./cmd/api
RUN CGO_ENABLED=0 go build -o /bin/worker ./cmd/worker

FROM alpine:3.18
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/api /bin/worker /usr/local/bin/
COPY --from=builder /app/configs /app/configs
WORKDIR /app
EXPOSE 8080 9093
# Default CMD for api; override in compose for worker
CMD ["/usr/local/bin/api"]

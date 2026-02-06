# 占位 Dockerfile（设计 struct.md 5）
# 后续可改为多阶段构建：build rag-platform 二进制后放入最小镜像
FROM golang:1.21-alpine AS builder
WORKDIR /app
# COPY go.mod go.sum ./
# RUN go mod download
# COPY . .
# RUN CGO_ENABLED=0 go build -o /rag-platform ./cmd/api

FROM alpine:3.18
# COPY --from=builder /rag-platform /usr/local/bin/
RUN echo "placeholder"
CMD ["echo", "replace with api binary"]

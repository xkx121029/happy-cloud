# 多阶段构建：产物为单一二进制，兼容 arm64
FROM golang:1.26-alpine AS builder
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o happy-cloud-backend .

FROM alpine:3.20
RUN apk add --no-cache tzdata ca-certificates && mkdir -p /data/happy-cloud
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=builder /build/happy-cloud-backend .
EXPOSE 8080
CMD ["./happy-cloud-backend"]

# Build stage
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://proxy.golang.org,direct
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /go-audio-api ./cmd/api

# Run stage
FROM alpine:3.18
RUN apk add --no-cache ca-certificates
WORKDIR /root/
COPY --from=builder /go-audio-api /go-audio-api
EXPOSE 8080
CMD ["/go-audio-api"]

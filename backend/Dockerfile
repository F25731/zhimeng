FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go mod tidy && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/control-api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/control-agent ./cmd/agent && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/control-reporter ./cmd/reporter && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/init-admin ./cmd/init-admin

FROM alpine:3.20
RUN apk add --no-cache ca-certificates docker-cli docker-cli-compose postgresql16-client && \
    addgroup -S control && adduser -S control -G control
WORKDIR /app
COPY --from=build /out/control-api /out/control-agent /out/control-reporter /out/init-admin ./
RUN mkdir -p /app/uploads && chown -R control:control /app/uploads
EXPOSE 8080

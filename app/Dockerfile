FROM golang:1.23-alpine AS builder

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN go build -trimpath -ldflags="-s -w" -o /out/http-server-projeto-korp .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /out/http-server-projeto-korp .

EXPOSE 8080
USER nobody:nobody
ENTRYPOINT ["./http-server-projeto-korp"]

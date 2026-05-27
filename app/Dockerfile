# syntax=docker/dockerfile:1.7
# =============================================================================
# Stage 1: Builder
# =============================================================================
FROM golang:1.23-alpine AS builder

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /src

COPY go.mod go.sum ./

# --mount=type=cache persiste o cache de módulos Go entre builds.
# Reduz tempo de rebuild de ~30s para ~2s quando só o código muda.
# O cache é isolado por escopo (uid do usuário) e não vaza entre projetos.
RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY main.go ./

RUN --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/http-server-projeto-korp \
    ./main.go

# =============================================================================
# Stage 2: Runtime
# =============================================================================
# distroless/static:nonroot — ~2MB, sem shell, sem pacotes, roda como UID 65532.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/http-server-projeto-korp /app/http-server-projeto-korp

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/app/http-server-projeto-korp"]

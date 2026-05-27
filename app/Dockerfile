# syntax=docker/dockerfile:1.7
# =============================================================================
# Stage 1: Builder
# =============================================================================
# Imagem oficial Go pinada. Alpine reduz a superfície durante o build sem
# perder funcionalidade (Go é estaticamente linkado).
FROM golang:1.23-alpine AS builder

# Boas práticas:
#   - CGO_ENABLED=0 garante binário estático (necessário pra rodar em distroless/static)
#   - GOOS/GOARCH explícitos evitam surpresas em builds multi-plataforma
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

WORKDIR /src

# Copia go.mod/go.sum antes do código fonte pra aproveitar o cache de layers
# do Docker quando só o código (sem deps) muda.
COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./

# Build flags:
#   -trimpath: remove paths absolutos do binário (reprodutibilidade + privacidade)
#   -ldflags "-s -w": strip de debug info, reduz tamanho do binário
RUN go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/http-server-projeto-korp \
    ./main.go

# =============================================================================
# Stage 2: Runtime
# =============================================================================
# distroless/static:nonroot é a menor imagem possível pra binário Go estático:
#   - ~2MB
#   - sem shell, sem apt, sem nada além de libc e certificados CA
#   - roda como UID 65532 (nonroot) por default
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/http-server-projeto-korp /app/http-server-projeto-korp

# Documenta a porta (não publica — quem publica é o compose/k8s).
EXPOSE 8080

# USER já vem como nonroot da imagem base; redundante mas explícito.
USER nonroot:nonroot

ENTRYPOINT ["/app/http-server-projeto-korp"]

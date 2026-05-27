// Package main implementa o http-server-projeto-korp.
//
// Responsabilidades:
//   - Servir GET /projeto-korp com JSON {nome, horario(UTC)}
//   - Expor /metrics no padrão Prometheus (RPS, latência, status codes)
//   - Expor /healthz para liveness/readiness probes
//   - Graceful shutdown em SIGINT/SIGTERM
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// projetoKorpResponse é o contrato de resposta do endpoint principal.
type projetoKorpResponse struct {
	Nome    string `json:"nome"`
	Horario string `json:"horario"`
}

// Métricas customizadas + as padrão do Go runtime (já vêm via promauto).
//
// Decisões:
//   - http_requests_total: counter com labels method/path/status -> base para RPS e taxa de erro
//   - http_request_duration_seconds: histogram -> permite calcular p50/p95/p99
//   - service_up: gauge -> "disponibilidade" exposta como métrica explícita (1=up)
//
// Buckets do histogram cobrem de 1ms a 10s, faixa típica de API HTTP.
var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total de requisições HTTP processadas, particionado por método, path e status.",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duração das requisições HTTP em segundos.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path"},
	)

	serviceUp = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "service_up",
			Help: "1 quando o serviço está operacional, 0 caso contrário.",
		},
	)
)

// statusRecorder embrulha http.ResponseWriter para capturar o status code,
// que o net/http não expõe nativamente para middlewares.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// instrument é o middleware que aplica as métricas a qualquer handler.
// path vem como parâmetro porque o net/http puro não tem route templating —
// se usasse chi/gorilla, daria pra extrair o pattern. Para o escopo, hardcode é ok.
func instrument(path string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(sr, r)

		duration := time.Since(start).Seconds()
		httpRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
		httpRequestsTotal.WithLabelValues(r.Method, path, strconv.Itoa(sr.status)).Inc()
	}
}

func projetoKorpHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := projetoKorpResponse{
		Nome:    "Projeto Korp",
		Horario: time.Now().UTC().Format(time.RFC3339Nano),
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("falha ao serializar resposta", "erro", err)
	}
}

func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Registry default do promauto já inclui métricas Go runtime + process via
	// collectors.NewGoCollector() / NewProcessCollector() implicitamente, mas
	// reforço para explicitar a intenção.
	prometheus.DefaultRegisterer.MustRegister(
		collectors.NewBuildInfoCollector(),
	)

	serviceUp.Set(1)

	mux := http.NewServeMux()
	mux.HandleFunc("/projeto-korp", instrument("/projeto-korp", projetoKorpHandler))
	mux.HandleFunc("/healthz", instrument("/healthz", healthzHandler))
	// /metrics não é instrumentado pra não poluir as próprias métricas com self-scrapes.
	mux.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,  // mitiga slowloris
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown: escuta SIGINT/SIGTERM, dá 10s pras conexões drenarem.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("servidor iniciado", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("erro fatal no servidor", "erro", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("sinal de shutdown recebido, drenando conexões")
	serviceUp.Set(0)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown forçado", "erro", err)
	}
	slog.Info("servidor encerrado limpo")
}

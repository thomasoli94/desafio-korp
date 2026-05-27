# Desafio Korp — Analista DevOps

Solução completa do desafio técnico: serviço HTTP em Go, containerização com Docker, proxy reverso NGINX, observabilidade com Prometheus + Grafana e provisionamento automatizado com Ansible.

> **TL;DR** — Rode `cd ansible && ansible-playbook site.yml --ask-become-pass` em um Ubuntu 22.04/24.04 limpo. O ambiente sobe sozinho e a validação HTTP é exibida no console no final.

---

## Sumário

- [Arquitetura](#arquitetura)
- [Stack e decisões técnicas](#stack-e-decisões-técnicas)
- [Como executar](#como-executar)
- [Validação manual](#validação-manual)
- [Estrutura do repositório](#estrutura-do-repositório)
- [Métricas expostas](#métricas-expostas)
- [Dashboard Grafana](#dashboard-grafana)
- [Decisões de segurança](#decisões-de-segurança)
- [Próximos passos](#próximos-passos)

---

## Arquitetura

```
                  ┌────────────┐
   curl :80 ──►   │   NGINX    │  ◄── proxy reverso, único ponto público
                  └─────┬──────┘
                        │ rede bridge: projeto-korp-net
                        ▼
              ┌─────────────────────┐
              │ http-server-projeto │  :8080  (não exposto ao host)
              │       -korp (Go)    │  ◄── expõe /projeto-korp, /metrics, /healthz
              └──────────┬──────────┘
                         │ scrape /metrics
                         ▼
                  ┌────────────┐         ┌────────────┐
                  │ Prometheus │ ◄────── │  Grafana   │  :3000
                  │   :9090    │ query   │ (dashboard │
                  └────────────┘         │  provisionado)
                                         └────────────┘
```

Toda a comunicação inter-container acontece na bridge `projeto-korp-net`. O host só enxerga as portas 80 (NGINX), 9090 (Prometheus) e 3000 (Grafana).

## Stack e decisões técnicas

| Componente | Versão | Por quê |
|---|---|---|
| Go | 1.23-alpine | LTS atual, alpine pra build enxuto |
| `prometheus/client_golang` | 1.20.5 | Lib oficial, padrão de mercado |
| Imagem runtime | distroless/static | ~2MB, sem shell, sem CVEs de pacotes |
| NGINX | 1.27-alpine | LTS, alpine reduz superfície |
| Prometheus | 2.54.1 | Última estável da série 2.x |
| Grafana | 11.2.0 | Provisionamento de dashboards estável |
| Ansible | 2.14+ | Padrão da indústria pra IaC procedural |

**Por que multi-stage Dockerfile?** O builder traz toolchain Go (~300MB); o runtime distroless tem só o binário estático (~10MB final). Em produção isso significa pulls mais rápidos, menos CVEs e menos superfície de ataque.

**Por que NGINX não passa `/metrics`?** Métricas Prometheus contêm informação sensível (paths internos, contadores que podem revelar comportamento). Em produção, `/metrics` fica em rede privada. Prometheus aqui acessa direto via DNS interno do Docker.

**Por que `service_up` como gauge se já tem `up` automático do Prometheus?** O `up` é externo (Prometheus consegue ou não scrape). `service_up` é interno (aplicação se declara saudável) — combinados detectam mais tipos de falha. Durante graceful shutdown, `service_up=0` antes do scrape falhar.

**Por que graceful shutdown?** Em Kubernetes ou autoscaling, pods recebem SIGTERM antes de SIGKILL. Sem handler, conexões em voo são abortadas, gerando 5xx no cliente. Padrão é drenar com timeout.

## Como executar

### Pré-requisitos

- Ubuntu 22.04 ou 24.04 (testado em ambos)
- `sudo` disponível pro usuário corrente
- Conexão à internet (pra apt e Docker Hub)
- Ansible instalado: `sudo apt install -y ansible`

### Provisionamento completo (Ansible)

```bash
git clone https://github.com/thomasoli94/desafio-korp.git
cd desafio-korp/ansible
ansible-playbook site.yml --ask-become-pass
```

Saída esperada no final:

```
==================================================
Ambiente Projeto Korp provisionado com sucesso!
==================================================
Endpoint: http://localhost:80/projeto-korp
Status:   HTTP 200
Resposta: {'nome': 'Projeto Korp', 'horario': '2026-05-27T18:45:23.123456789Z'}
--------------------------------------------------
Grafana:    http://localhost:3000 (admin/admin)
Prometheus: http://localhost:9090
==================================================
```

### Execução manual (sem Ansible, pra debug)

```bash
docker compose up -d --build
curl http://localhost:80/projeto-korp
```

## Validação manual

```bash
# Endpoint principal (deve retornar JSON com nome e horario UTC)
curl -s http://localhost/projeto-korp | jq

# Healthcheck
curl -s http://localhost/healthz

# Métricas raw (acessível só na rede interna; via docker exec pra ver)
docker exec prometheus-projeto-korp wget -qO- http://http-server-projeto-korp:8080/metrics | head -30

# Targets do Prometheus
curl -s http://localhost:9090/api/v1/targets | jq '.data.activeTargets[] | {job: .labels.job, health: .health}'

# Gerar carga pra ver as métricas se moverem
for i in {1..200}; do curl -s http://localhost/projeto-korp > /dev/null; done
```

## Estrutura do repositório

```
.
├── app/                              # Aplicação Go
│   ├── main.go                       # Server HTTP + métricas + shutdown
│   ├── go.mod / go.sum
│   ├── Dockerfile                    # Multi-stage com distroless
│   └── .dockerignore
├── nginx/
│   └── http-server-projeto-korp.conf # Proxy reverso
├── prometheus/
│   └── prometheus.yml                # Scrape config
├── grafana/
│   ├── provisioning/
│   │   ├── datasources/datasource.yml
│   │   └── dashboards/dashboards.yml
│   └── dashboards/
│       └── http-server-projeto-korp-dashboard.json
├── docker-compose.yml                # Orquestração local
├── ansible/
│   ├── ansible.cfg
│   ├── site.yml                      # Playbook principal
│   ├── inventory/hosts.ini
│   └── roles/
│       ├── docker/                   # Instala engine + cria rede
│       ├── app/                      # Sobe a stack
│       └── monitoring/               # Valida observabilidade
└── README.md
```

## Métricas expostas

A aplicação expõe no `/metrics`:

| Métrica | Tipo | Labels | Para que serve |
|---|---|---|---|
| `service_up` | gauge | — | Disponibilidade declarada pela própria app |
| `http_requests_total` | counter | method, path, status | Volume de requisições, taxa de erro |
| `http_request_duration_seconds` | histogram | method, path | Latência p50/p95/p99 |
| `go_*`, `process_*` | various | — | Runtime do Go (mem, GC, goroutines) — automáticas |

**Disponibilidade** é coberta por duas perspectivas:
- `service_up` (interna): a aplicação está se declarando saudável?
- `up{job="http-server-projeto-korp"}` (externa): o Prometheus consegue scrape?

**Volume de requisições** sai de `rate(http_requests_total[1m])` no Grafana.

## Dashboard Grafana

O dashboard `Projeto Korp - Visão Geral` é provisionado automaticamente. Painéis:

1. **Disponibilidade do Serviço** — UP/DOWN colorido
2. **Uptime últimos 5min** — % do tempo que o scrape foi bem-sucedido
3. **RPS atual** — requisições por segundo agora
4. **Error Rate** — taxa de 5xx
5. **Volume por Endpoint** — timeseries de RPS por path
6. **Latência p50/p95/p99** — distribuição de duração
7. **Status Codes** — barras empilhadas por status
8. **Memória Go** — heap e RSS pra detectar leaks

Acesso: http://localhost:3000 → admin/admin → "Projeto Korp" folder.

## Decisões de segurança

- **distroless/static** elimina ~95% das CVEs típicas de imagem base
- **USER nonroot** em todos os containers customizados
- **`/metrics` não exposto via NGINX** — fica só na rede interna
- **Resource limits** no compose previnem runaway local
- **Volumes `:ro`** onde aplicável (config files não precisam de write)
- **`host_key_checking = False`** no Ansible é só pra dev local; em prod, manter `True`

Em **ambiente real de produção**, eu adicionaria:
- Secrets do Grafana via variável de ambiente do CI ou Vault, nunca hardcoded
- TLS no NGINX (Let's Encrypt via certbot ou cert-manager se K8s)
- Network policies se Kubernetes
- Image scanning (Trivy/Grype) no pipeline
- SBOM gerado no build

## Próximos passos (extensões naturais)

- [ ] Alertmanager + alertas Prometheus (ex: error_rate > 1%, p99 > 500ms)
- [ ] Tracing distribuído (OpenTelemetry)
- [ ] Logs estruturados pro Loki
- [ ] Helm chart pra deploy em Kubernetes
- [ ] GitHub Actions: lint, test, build, push de imagem
- [ ] Testes de integração com testcontainers-go

---

**Autor:** Thomas Medeiros
**Contato:** thomassilvamedeiros@gmail.com

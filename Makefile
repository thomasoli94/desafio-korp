# Atalhos pra operação do projeto. 'make help' lista tudo.

.PHONY: help up down logs status test clean ansible-run lint

help: ## Mostra esta ajuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

up: ## Sobe a stack via docker-compose
	docker compose up -d --build

down: ## Derruba a stack (preserva volumes)
	docker compose down

clean: ## Derruba a stack e apaga volumes
	docker compose down -v

logs: ## Mostra logs de todos os serviços
	docker compose logs -f

status: ## Status dos containers
	docker compose ps

test: ## Valida endpoint principal
	@echo "Testando /projeto-korp..."
	@curl -sf http://localhost/projeto-korp | jq . || echo "FALHA"
	@echo "Testando /healthz..."
	@curl -sf http://localhost/healthz | jq . || echo "FALHA"

load: ## Gera carga (200 requests) pra ver métricas se moverem
	@echo "Gerando carga..."
	@for i in $$(seq 1 200); do curl -s http://localhost/projeto-korp > /dev/null; done
	@echo "Pronto. Veja em http://localhost:3000"

ansible-run: ## Roda o playbook Ansible (provisionamento completo)
	cd ansible && ansible-playbook site.yml --ask-become-pass

ansible-check: ## Roda o playbook em modo --check (dry-run)
	cd ansible && ansible-playbook site.yml --check --ask-become-pass

lint: ## Lint do compose e dos playbooks
	docker compose config -q && echo "docker-compose.yml: OK"
	cd ansible && ansible-playbook site.yml --syntax-check

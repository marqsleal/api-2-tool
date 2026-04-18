COMPOSE = docker compose -f docker-compose.yml
API_BASE_URL ?= http://localhost:8080

.PHONY: up down build logs ps seed rebuild-chat rebuild-api restart

up:
	@$(COMPOSE) up -d --build
	@$(MAKE) seed

down:
	@$(COMPOSE) down

build:
	@$(COMPOSE) build

logs:
	@$(COMPOSE) logs -f

ps:
	@$(COMPOSE) ps

seed:
	@echo "Aguardando API em $(API_BASE_URL)/health..."
	@until curl --silent --fail "$(API_BASE_URL)/health" >/dev/null; do \
		sleep 1; \
	done
	@API_BASE_URL="$(API_BASE_URL)" bash scripts/001__cadastro-todas-apis.sh

rebuild-chat:
	@$(COMPOSE) up -d --build chatbot-api chatbot-ui

rebuild-api:
	@$(COMPOSE) up -d --build api-2-tool

restart:
	@$(COMPOSE) restart

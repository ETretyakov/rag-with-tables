BINARY := bin/server
MODULE  := github.com/ETretyakov/rag-with-tables

.PHONY: up down logs run build test lint tidy

## ─── Infrastructure ────────────────────────────────────────────────────────────

up:          ## Поднять Qdrant + MinIO в фоне
	docker compose up -d

down:        ## Остановить и удалить контейнеры (данные сохраняются)
	docker compose down

logs:        ## Следить за логами контейнеров
	docker compose logs -f

## ─── Application ──────────────────────────────────────────────────────────────

run:         ## Запустить сервер локально (читает .env)
	CGO_ENABLED=1 go run ./cmd/server

build:       ## Собрать бинарник в bin/server
	CGO_ENABLED=1 go build -ldflags="-s -w" -o $(BINARY) ./cmd/server

## ─── Development ──────────────────────────────────────────────────────────────

test:        ## Запустить тесты с -race
	CGO_ENABLED=1 go test -race ./...

lint:        ## Прогнать golangci-lint
	golangci-lint run ./...

tidy:        ## Привести go.mod/go.sum в порядок
	go mod tidy

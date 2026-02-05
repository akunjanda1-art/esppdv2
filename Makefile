SHELL := /bin/sh

.PHONY: up down build logs fmt test tidy

up:
	docker compose up -d --build

down:
	docker compose down -v

build:
	docker compose build

logs:
	docker compose logs -f --tail=200

fmt:
	@for d in services/*; do (cd $$d && gofmt -w .); done

test:
	@for d in services/*; do (cd $$d && go test ./...); done

tidy:
	@for d in services/*; do (cd $$d && go mod tidy); done

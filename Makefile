.PHONY: run build test vet migrate db-up db-down

run:
	go run ./cmd/api

build:
	go build -o bin/api ./cmd/api

test:
	go test ./...

vet:
	go vet ./...

migrate:
	# Requires DATABASE_URL. Uses psql against migrations/ in lexical order.
	@for f in migrations/*.up.sql; do \
		echo "applying $$f"; psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -f "$$f"; \
	done

db-up:
	docker compose up -d db

db-down:
	docker compose down

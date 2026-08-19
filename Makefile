MIGRATE_URL ?= $(DATABASE_URL)

.PHONY: run build test test-race lint tidy migrate-up migrate-down migrate-new docker

run:
	go run ./cmd/web

build:
	go build -o bin/web ./cmd/web

test:
	go test ./... -count=1

# Needs a C compiler (gcc). On Windows install it via mingw-w64 or msys2.
test-race:
	CGO_ENABLED=1 go test ./... -race -count=1

lint:
	go vet ./...
	gofmt -l .

tidy:
	go mod tidy

# Requires golang-migrate: https://github.com/golang-migrate/migrate
migrate-up:
	migrate -database "$(MIGRATE_URL)" -path db/migrations up

migrate-down:
	migrate -database "$(MIGRATE_URL)" -path db/migrations down 1

migrate-new:
	migrate create -ext sql -dir db/migrations -seq $(name)

docker:
	docker build -t audiax-backend .

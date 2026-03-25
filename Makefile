BINARY=scheduler
BUILD_DIR=bin

.PHONY: all build run test clean docker-build docker-up docker-down docker-logs tidy

all: build

## Build the binary locally
build:
	go build -o $(BUILD_DIR)/$(BINARY) .

## Run locally
run:
	go run .

## Run tests
test:
	go test ./...

## Tidy dependencies
tidy:
	go mod tidy

## Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)

## Build Docker image
docker-build:
	docker-compose build

## Start all services (app + redis)
docker-up:
	docker-compose up --build -d

## Stop all services
docker-down:
	docker-compose down

## View logs
docker-logs:
	docker-compose logs -f

## Stop and remove volumes
docker-clean:
	docker-compose down -v

# Makefile for Restaurant API

# Variables
APP_NAME=restaurant-api
VERSION=v1.0.0
DOCKER_IMAGE=$(APP_NAME):$(VERSION)
PORT=8000

# Colors for output
RED=\033[0;31m
GREEN=\033[0;32m
YELLOW=\033[1;33m
NC=\033[0m # No Color

.PHONY: help build run test clean docker-build docker-run docker-stop deps fmt vet lint

# Default target
help: ## Display this help message
	@echo "$(GREEN)Restaurant API - Makefile Commands$(NC)"
	@echo ""
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "$(YELLOW)%-15s$(NC) %s\n", $$1, $$2}' $(MAKEFILE_LIST)

deps: ## Download dependencies
	@echo "$(GREEN)Downloading dependencies...$(NC)"
	go mod download
	go mod tidy

fmt: ## Format Go code
	@echo "$(GREEN)Formatting Go code...$(NC)"
	go fmt ./...

vet: ## Run go vet
	@echo "$(GREEN)Running go vet...$(NC)"
	go vet ./...

lint: ## Run golangci-lint (requires golangci-lint to be installed)
	@echo "$(GREEN)Running golangci-lint...$(NC)"
	golangci-lint run

build: deps fmt vet ## Build the application
	@echo "$(GREEN)Building $(APP_NAME)...$(NC)"
	go build -o bin/$(APP_NAME) .

run: ## Run the application locally
	@echo "$(GREEN)Starting $(APP_NAME) on port $(PORT)...$(NC)"
	@echo "$(YELLOW)Press Ctrl+C to stop$(NC)"
	go run main.go

dev: ## Run in development mode with air (requires air to be installed)
	@echo "$(GREEN)Starting development server with hot reload...$(NC)"
	air

test: ## Run tests
	@echo "$(GREEN)Running tests...$(NC)"
	go test -v ./...

test-coverage: ## Run tests with coverage
	@echo "$(GREEN)Running tests with coverage...$(NC)"
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report generated: coverage.html$(NC)"

clean: ## Clean build artifacts
	@echo "$(GREEN)Cleaning build artifacts...$(NC)"
	rm -rf bin/
	rm -f coverage.out coverage.html

# Docker commands
docker-build: ## Build Docker image
	@echo "$(GREEN)Building Docker image: $(DOCKER_IMAGE)$(NC)"
	docker build -t $(DOCKER_IMAGE) .

docker-run: docker-build ## Run Docker container
	@echo "$(GREEN)Running Docker container on port $(PORT)...$(NC)"
	docker run -d --name $(APP_NAME) -p $(PORT):$(PORT) $(DOCKER_IMAGE)
	@echo "$(GREEN)Container started. Access at http://localhost:$(PORT)$(NC)"

docker-stop: ## Stop and remove Docker container
	@echo "$(GREEN)Stopping Docker container...$(NC)"
	-docker stop $(APP_NAME)
	-docker rm $(APP_NAME)

docker-logs: ## Show Docker container logs
	@echo "$(GREEN)Showing Docker logs...$(NC)"
	docker logs -f $(APP_NAME)

# Docker Compose commands
compose-up: ## Start services with docker-compose
	@echo "$(GREEN)Starting services with docker-compose...$(NC)"
	docker-compose up -d
	@echo "$(GREEN)Services started. Access at http://localhost:$(PORT)$(NC)"

compose-down: ## Stop docker-compose services
	@echo "$(GREEN)Stopping docker-compose services...$(NC)"
	docker-compose down

compose-logs: ## Show docker-compose logs
	@echo "$(GREEN)Showing docker-compose logs...$(NC)"
	docker-compose logs -f

compose-build: ## Build docker-compose services
	@echo "$(GREEN)Building docker-compose services...$(NC)"
	docker-compose build

# Development tools
install-tools: ## Install development tools
	@echo "$(GREEN)Installing development tools...$(NC)"
	go install github.com/cosmtrek/air@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# API testing
test-api: ## Test API endpoints (requires curl)
	@echo "$(GREEN)Testing API endpoints...$(NC)"
	@echo "$(YELLOW)Testing root endpoint...$(NC)"
	curl -s http://localhost:$(PORT)/ | jq .
	@echo ""
	@echo "$(YELLOW)Testing categories endpoint...$(NC)"
	curl -s http://localhost:$(PORT)/api/categories | jq .

# Deployment
deploy-staging: ## Deploy to staging (placeholder)
	@echo "$(YELLOW)Deploy to staging - Not implemented$(NC)"

deploy-production: ## Deploy to production (placeholder)
	@echo "$(RED)Deploy to production - Not implemented$(NC)"

# Database operations (for future use)
db-migrate: ## Run database migrations (placeholder)
	@echo "$(YELLOW)Database migrations - Not implemented$(NC)"

db-seed: ## Seed database with test data (placeholder)
	@echo "$(YELLOW)Database seeding - Not implemented$(NC)"

# Quality checks
check: fmt vet lint test ## Run all quality checks

# Security
security-scan: ## Run security scan (requires gosec)
	@echo "$(GREEN)Running security scan...$(NC)"
	gosec ./...

# Benchmarks
benchmark: ## Run benchmarks
	@echo "$(GREEN)Running benchmarks...$(NC)"
	go test -bench=. -benchmem ./...

# Generate documentation
docs: ## Generate documentation (placeholder)
	@echo "$(YELLOW)Documentation generation - Not implemented$(NC)"

# Release
release: clean build test ## Prepare release
	@echo "$(GREEN)Preparing release $(VERSION)...$(NC)"
	@echo "$(GREEN)Release ready!$(NC)"
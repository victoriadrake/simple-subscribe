SHELL := /bin/bash
.POSIX:
.PHONY: build update

.PHONY: help
help: ## Show this help
	@egrep -h '\s##\s' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary
	GOOS=linux go build

dynamodb: ## Create the DynamoDB table
	./scripts/create-table.sh

update: ## Upload function code to Lambda and update Lambda environment variables
	./scripts/update-lambda.sh
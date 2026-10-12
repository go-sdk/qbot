MAKEFLAGS       += --no-print-directory

.PHONY: tidy
tidy:					##@ Tidy go.mod and go.sum.
	@go mod tidy

.PHONY: lint
lint: tidy				##@ Lint all packages.
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --timeout 5m && \
		echo "done."; \
	else \
		echo "golangci-lint is not installed. Please install it from https://github.com/golangci/golangci-lint"; \
		exit 1; \
	fi

.PHONY: test
test: tidy				##@ Test all packages.
	@if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --format testname --format-icons text -- -race -count 1 -failfast -v ./...; \
	else \
		go test -race -count 1 -failfast -v ./...; \
	fi

.PHONY: test-network
test-network: tidy			##@ Test real QQ access token retrieval using QBOT_* environment variables.
	@QBOT_NETWORK_TESTS=1 go test -count 1 -failfast -v -run '^TestNetworkQQAccessToken$$' ./...

.PHONY: test-send
test-send: tidy				##@ Send one real QQ private text message using QBOT_* environment variables.
	@QBOT_SEND_TESTS=1 go test -count 1 -failfast -v -run '^TestNetworkQQSendTextFromEnv$$' ./...

.PHONY: test-send-group
test-send-group: tidy			##@ Send one real QQ group text message using QBOT_* environment variables.
	@QBOT_GROUP_SEND_TESTS=1 go test -count 1 -failfast -v -run '^TestNetworkQQSendGroupTextFromEnv$$' ./...

.PHONY: help
help:					##@ (Default) Show help.
	@printf "\nUsage: make <command>\n"
	@grep -F -h "##@" $(MAKEFILE_LIST) | grep -F -v grep -F | sed -e 's/\\$$//' | awk 'BEGIN {FS = ":*[[:space:]]*##@[[:space:]]*"}; \
	{ \
		if($$2 == "") \
			pass; \
		else if($$0 ~ /^#/) \
			printf "\n%s\n", $$2; \
		else if($$1 == "") \
			printf "     %-20s%s\n", "", $$2; \
		else \
			printf "\n    \033[34m%-20s\033[0m %s\n", $$1, $$2; \
	}'
	@printf "\n"

.DEFAULT_GOAL := help

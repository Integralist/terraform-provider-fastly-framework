.PHONY: all build clean default docs help test testacc testacc_debug testacc_trace

TEST_COMMAND ?= go test ## Enables support for tools such as https://github.com/rakyll/gotest

build: lint ## Compile development build for local testing of the provider.
	@sh -c "'./scripts/local-build.sh'"
	@go install .

clean: ## Clean Fastly services that have leaked when running acceptance tests
	@if [ "$(SILENCE)" != "true" ]; then \
		printf "WARNING: This will destroy infrastructure. Use only in development accounts.\n\n"; \
		fi
	@fastly service list --token ${FASTLY_API_TOKEN} | grep -E '^tf\-' | awk '{print $$2}' | xargs -I % fastly service delete --token ${FASTLY_API_KEY} -f -s %

deps_update: ## Update all go.mod dependencies to the latest versions
	go get -u ./...

docs: ## Generate documentation
	go generate ./...

lint: ## Run golangci-lint
	golangci-lint run --verbose

nilaway: ## Run nilaway
	@nilaway ./...

testacc: ## Run acceptance tests
	TF_ACC=1 $(TEST_COMMAND) ./... -v $(TESTARGS) -timeout 120m

testacc_debug: ## Run acceptance tests with debug provider logs
	TF_LOG_PROVIDER_FASTLY=DEBUG make testacc

testacc_trace: ## Run acceptance tests with trace provider logs
	TF_LOG_PROVIDER_FASTLY=TRACE make testacc

help: ## Display this help message
	@printf "Targets\n"
	@grep -h -E '^[0-9a-zA-Z_.-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-22s\033[0m %s\n", $$1, $$2}'
	@printf "\nDefault target\n"
	@printf "\033[36m%s\033[0m" $(.DEFAULT_GOAL)
	@printf "\n\nMake Variables\n"
	@(grep -h -E '^[0-9a-zA-Z_.-]+\s[:?]?=.*? ## .*$$' $(MAKEFILE_LIST) || true) | sort | awk 'BEGIN {FS = "[:?]?=.*?## "}; {printf "\033[36m%-25s\033[0m %s\n", $$1, $$2}'

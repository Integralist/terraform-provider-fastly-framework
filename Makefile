.PHONY: all build clean default docs test testacc testacc_debug testacc_trace

# Enables support for tools such as https://github.com/rakyll/gotest
TEST_COMMAND ?= go test

# Compile development build for local testing of the provider.
build:
	@sh -c "'./scripts/local-build.sh'"
	@go install .

# Clean Fastly services that have leaked when running acceptance tests
clean:
	@if [ "$(SILENCE)" != "true" ]; then \
		echo "WARNING: This will destroy infrastructure. Use only in development accounts."; \
		fi
	@fastly service list --token ${FASTLY_API_TOKEN} | grep -E '^tf\-' | awk '{print $$2}' | xargs -I % fastly service delete --token ${FASTLY_API_KEY} -f -s %

# Define the default Make target if none is specified via the command-line.
default: testacc

# Generate documentation
docs:
	go generate ./...

# Run acceptance tests
testacc:
	TF_ACC=1 $(TEST_COMMAND) ./... -v $(TESTARGS) -timeout 120m

# Run acceptance tests with debug provider logs
testacc_debug:
	TF_LOG_PROVIDER_FASTLY=DEBUG make testacc

# Run acceptance tests with trace provider logs
testacc_trace:
	TF_LOG_PROVIDER_FASTLY=TRACE make testacc

default: testacc

# Enables support for tools such as https://github.com/rakyll/gotest
TEST_COMMAND ?= go test

# Run acceptance tests
testacc:
	TF_ACC=1 $(TEST_COMMAND) ./... -v $(TESTARGS) -timeout 120m

# Run acceptance tests with trace provider logs
testacc_trace:
	TF_LOG_PROVIDER_FASTLY=TRACE make testacc

# Run acceptance tests with debug provider logs
testacc_debug:
	TF_LOG_PROVIDER_FASTLY=DEBUG make testacc

# Generate documentation
docs:
	go generate ./...

# Clean Fastly services that have leaked when running acceptance tests
clean:
	@if [ "$(SILENCE)" != "true" ]; then \
		echo "WARNING: This will destroy infrastructure. Use only in development accounts."; \
		fi
	@fastly service list --token ${FASTLY_API_TOKEN} | grep -E '^tf\-' | awk '{print $$2}' | xargs -I % fastly service delete --token ${FASTLY_API_KEY} -f -s %

.PHONY: all clean default docs test testacc testacc_debug testacc_trace

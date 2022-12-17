default: testacc

# Enables support for tools such as https://github.com/rakyll/gotest
TEST_COMMAND ?= go test

# Run acceptance tests
testacc:
	TF_ACC=1 $(TEST_COMMAND) ./... -v $(TESTARGS) -timeout 120m

# Run acceptance tests with trace logs
testacc_logs:
	TF_LOG_PROVIDER_FASTLY=TRACE TF_ACC=1 $(TEST_COMMAND) ./... -v $(TESTARGS) -timeout 120m

# Generate documentation
docs:
	go generate ./...

.PHONY: all clean default docs test testacc testacc_logs

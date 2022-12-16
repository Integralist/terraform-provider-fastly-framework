default: testacc

# Enables support for tools such as https://github.com/rakyll/gotest
TEST_COMMAND ?= go test

# Run acceptance tests
testacc:
	TF_ACC=1 $(TEST_COMMAND) ./... -v $(TESTARGS) -timeout 120m

.PHONY: all clean default test testacc

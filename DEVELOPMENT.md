# DEVELOPMENT

If you wish to work on the provider, you'll first need [Go](https://go.dev/) installed on your machine (see [Requirements](#requirements) below).

To generate or update documentation, run `go generate` (or `make docs`).

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.21

## Building

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To manually test the provider (e.g. using the provider with your own Terraform configuration), run `make build`, which runs `go install` but also produces a `~/.terraformrc` that enables the Terraform CLI to use the local build version.

## Testing

To run the full suite of Acceptance tests, use one of the available Makefile targets:

- `make testacc`
- `make testacc_debug`
- `make testacc_trace`

To run a specific test, set `TESTARGS`:

```shell
TESTARGS="-run=TestAccResourceServiceVCL" make testacc
```

> **NOTE:** Acceptance tests create real resources, and often cost money to run.

## Logging Practices

We use `tflog.Debug()` for describing important operational details like milestones in logic. It often describes behaviors that may be confusing even though they are correct.

We use `tflog.Trace()` for describing the lowest level operational details, such as intra-function steps or raw data and errors.

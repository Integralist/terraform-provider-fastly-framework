# Fastly Terraform Provider

> 🚨 This provider is WIP (Work in Progress).

The Fastly Terraform Provider interacts with most facets of the [Fastly API](https://developer.fastly.com/reference/api) and uses the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework).

## Using the provider

Consumers should refer to the [EXAMPLES](./examples/)

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.21

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

```shell
go install
```

## Developing the Provider

We document issues with the provider in [`DEVELOPMENT.md`](./DEVELOPMENT.md).

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To manually test the provider (e.g. using the provider with your own Terraform configuration), run `make build`, which runs `go install` but also produces a `~/.terraformrc` that enables the Terraform CLI to use the local build version.

To generate or update documentation, run `go generate` (or `make docs`).

To run the full suite of Acceptance tests, run `make testacc`.

> **NOTE:** Acceptance tests create real resources, and often cost money to run.

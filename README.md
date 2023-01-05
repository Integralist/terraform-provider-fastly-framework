# Fastly Terraform Provider

The Fastly Terraform Provider interacts with most facets of the [Fastly API](https://developer.fastly.com/reference/api) and uses the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework).

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.18

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

```shell
go install
```

## Using the provider

TODO

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `go generate` (or `make docs`).

To run the full suite of Acceptance tests, run `make testacc`.

> **NOTE:** Acceptance tests create real resources, and often cost money to run.

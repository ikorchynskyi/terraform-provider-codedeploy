# Terraform Provider Codedeploy

This repository is a [Terraform](https://www.terraform.io) provider for CodeDeploy deployment execution.

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.4
- [Go](https://golang.org/doc/install) >= 1.20

## Build provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the `make install` command

## Local release build

```shell
$ go install github.com/goreleaser/goreleaser@latest
```

```shell
$ make release
```

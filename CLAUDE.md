# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

`graph-cli` is a Go CLI tool (in early development) for interacting with the Microsoft Azure Graph API. It is built with [Cobra](https://github.com/spf13/cobra) for command structure and [Viper](https://github.com/spf13/viper) for configuration management.

It uses Microsoft's [Graph SDK for Go](github.com/microsoftgraph/msgraph-sdk-go) and [Azure Authentication SDK for Go](github.com/microsoft/kiota-authentication-azure-go)

## Commands

```bash
# Build
go build -o graph-cli .

# Run
go run main.go [command]

# Test
go test ./...

# Run a single test
go test ./cmd/... -run TestName

# Add a new subcommand (uses cobra-cli)
cobra-cli add <command-name>

# Lint (requires golangci-lint)
golangci-lint run
```

## Architecture

- `main.go` — entry point; calls `cmd.Execute()`
- `cmd/root.go` — defines the root `graph-cli` command, registers the `--config` persistent flag, and initializes Viper config from `$HOME/.graph-cli.yaml`
- New subcommands go in `cmd/` as separate files, each registering themselves onto `rootCmd` via their `init()` function

## Configuration

Viper looks for `$HOME/.graph-cli.yaml` by default. A custom path can be passed with `--config <path>`. Environment variables are automatically read if they match a config key.

## Cobra CLI scaffolding

`.cobra.yaml` configures `cobra-cli` defaults (author, license, Viper integration). When adding commands with `cobra-cli add`, the generated file will be placed in `cmd/` and must be wired to `rootCmd` in its `init()`.

# CATA.Storage/Flow

CATA.Storage is desigined based on Flow storage node. It could be regarded as the parallel chain of Flow. Flow is a fast, secure, and developer-friendly blockchain built to support NFT application. CATA.Storage also Inherited these excellent characteristics.


<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

## Table of Contents

- [Getting started](#getting-started)
- [Documentation](#documentation)
- [Installation](#installation)
    - [Clone Repository](#clone-repository)
    - [Install Dependencies](#install-dependencies)
- [Development Workflow](#development-workflow)
    - [Testing](#testing)
    - [Building](#building)
    - [Local Network](#local-network)
    - [Code Generation](#code-generation)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Getting started

- To install all dependencies and tools, see the [project setup](#installation) guide
- To dig into more documentation about Flow, see the [documentation](#documentation)
- To learn how to contribute, see the [contributing guide](/CONTRIBUTING.md)
- To see information on developing Flow, see the [development workflow](#development-workflow)

## Documentation

You can find an overview of the Flow architecture on the [documentation website](https://www.onflow.org/primer).

Development on Flow is divided into work streams. Each work stream has a home directory containing high-level
documentation for the stream, as well as links to documentation for relevant components used by that work stream.

The following table lists all work streams and links to their home directory and documentation:

| Work Stream        | Home directory                             |
|--------------------|--------------------------------------------|
| Access Node        | [/cmd/access](/cmd/access)                 |
| Collection Node    | [/cmd/collection](/cmd/collection)         |
| Consensus Node     | [/cmd/consensus](/cmd/consensus)           |
| Execution Node     | [/cmd/execution](/cmd/execution)           |
| Verification Node  | [/cmd/verification](/cmd/verification)     |
| Observer Service   | [/cmd/observer](/cmd/observer)             |
| HotStuff           | [/consensus/hotstuff](/consensus/hotstuff) |
| Storage/cata.storage  | [/storage](/storage)                       |
| Ledger             | [/ledger](/ledger)                         |
| Networking         | [/network](/network/)                      |
| Cryptography       | [/crypto](/crypto)                         |

## Installation

### Clone Repository

- Clone this repository
- Clone this repository's submodules:

    ```bash
    git submodule update --init --recursive
    ```

### Install Dependencies

- Install [Go](https://golang.org/doc/install) (Flow supports Go 1.18 and later)
- Install [CMake](https://cmake.org/install/), which is used for building the crypto library
- Install [Docker](https://docs.docker.com/get-docker/), which is used for running a local network and integration tests
- Make sure the [`GOPATH`](https://golang.org/cmd/go/#hdr-GOPATH_environment_variable) and `GOBIN` environment variables
  are set, and `GOBIN` is added to your path:

    ```bash
    export GOPATH=$(go env GOPATH)
    export GOBIN=$GOPATH/bin
    export PATH=$PATH:$GOBIN
    ```

  Add these to your shell profile to persist them for future runs.
- Then, run the following command:

    ```bash
    make install-tools
    ```

At this point, you should be ready to build, test, and run Flow! 🎉

Note: Whenever the crypto module version imported by "go.mod" is updated to a version that was never locally imported before, the crypto dependency needs to be set-up. If not, you should notice errors about "relic" or "crypto". Run the following command to set-up the new module version:

```bash
make crypto_setup_gopath
```

## Development Workflow

### Testing

Flow has a unit test suite and an integration test suite. Unit tests for a module live within the module they are
testing. Integration tests live in `integration/tests`.

Run the unit test suite:

```bash
make test
```

Run the integration test suite:

```bash
make integration-test
```

### Building

The recommended way to build and run Flow for local development is using Docker.

Build a Docker image for all nodes:

```bash
make docker-build-flow
```

Build a Docker image for a particular node role (replace `$ROLE` with `collection`, `consensus`, etc.):

```bash
make docker-build-$ROLE
```

### Local Network

A local version of the network can be run for manual testing and integration. See
the [Local Network Guide](/integration/localnet/README.md) for instructions.

### Code Generation

Generated code is kept up to date in the repository, so should be committed whenever it changes.

Run all code generators:

```bash
make generate
```

Generate protobuf stubs:

```bash
make generate-proto
```

Generate OpenAPI schema models:

```bash
make generate-openapi
```

Generate mocks used for unit tests:

```bash
make generate-mocks
```

---
sidebar_position: 1
---

# Create an Application With Monomer

In this tutorial, you will learn how to create a new Monomer L2 application.
Before starting this tutorial, we recommend reading through the [Overview](/overview) and [Learn](/category/learn) sections in the documentation to familiarize yourself with the Monomer architecture and concepts.

## Create a New Monomer Application

We'll use the [monogen](https://github.com/polymerdao/monomer/tree/main/monogen) tool to bootstrap a new Monomer L2 application.

To generate a new Monomer application with the default configuration, navigate to the parent directory of where you want to store your application and run the following command:

```bash
rm -rf ~/.testapp \
  && rm -rf testapp \
  && go run github.com/polymerdao/monomer/cmd/monogen@v0.1.3 \
  && cd testapp \
  && ./setup-helper.sh
```

To modify the default configuration, you can pass the following flags to the `go run github.com/polymerdao/monomer/cmd/monogen` command:

```
--address-prefix string   address prefix (default "cosmos")
--app-dir-path string     project directory (default "./testapp")
--gomod-path string       go module path (default "github.com/testapp/testapp")
```

## Build and Run

To build the application, run one of the following commands from the `testapp` directory:

If using a Go version `<1.23.0`, run:

```bash
go build -o testappd ./cmd/testappd
```

If using a Go version `>=1.23.0`, run:

```bash
go build -ldflags=-checklinkname=0 -o testappd ./cmd/testappd
````

Now that our application is configured, we can start the Monomer application by running the following command.

```bash
./testappd monomer start --minimum-gas-prices 0.01wei --monomer.dev-start --api.enable
````

Congratulations! You've successfully integrated Monomer into your Cosmos SDK application.

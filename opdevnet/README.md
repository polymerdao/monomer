The `secrets.go` file is copied from the Optimism e2e tests [here](https://github.com/ethereum-optimism/optimism/blob/24a8d3e06e61c7a8938dfb7a591345a437036381/op-e2e/e2eutils/secrets.go).
Normally, we would import the definitions.
Unfortunately, that would create a transitive dependency on the Optimism e2e tests' [`config` package](https://github.com/ethereum-optimism/optimism/blob/24a8d3e06e61c7a8938dfb7a591345a437036381/op-e2e/config), which prints to stdout and messes with command line flags in an `init` function.

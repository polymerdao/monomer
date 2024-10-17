# monogen

We want this to be a separate go module so it can have a `go.mod` without replace directives, permitting a nice `go run` developer experience.
Ideally, the `github.com/polymerdao/monomer/monogen` package would be a module, but we created that before thinking it would be a module.
We can't retroactively make that package a module since that would cause a the top-level `github.com/polymerdao/monomer/monogen` package to be in two modules (past versions of the `github.com/polymerdao/monomer` module and current versions of the hypothetical `github.com/polyerdao/monomer/monogen` module).
As a result, we settle for creating a separate `github.com/polymerdao/monomer/cmd/monogen` module.

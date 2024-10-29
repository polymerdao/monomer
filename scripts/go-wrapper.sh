#!/bin/bash
set -e
# Because we transitively depend on github.com/fjl/memsize, we need to disable checklinkname in go1.23.0 and higher.
goVersion=$(go env GOVERSION)
if [[ $goVersion > go1.23.0 || $goVersion == go1.23.0 ]]; then
    GOFLAGS+=' -ldflags=-checklinkname=0 '
fi

export GOFLAGS
go "$@"

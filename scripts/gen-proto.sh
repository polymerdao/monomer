#!/bin/bash

set -e

SCRIPTS_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
MONOMER_DIR=$(cd "$SCRIPTS_DIR/.." && pwd)
GEN_DIR=$(cd "$MONOMER_DIR/gen" && pwd)
ROLLUP_DIR=$(cd "$MONOMER_DIR/x/rollup" && pwd)

# generate cosmos proto code
buf generate

# move the generated rollup module message types to the x/rollup module
cp -r $GEN_DIR/rollup/v1/* $ROLLUP_DIR/types
rm -rf $GEN_DIR/rollup/v1

# TODO: move the testapp module message types to the testapp/x/testmodule module

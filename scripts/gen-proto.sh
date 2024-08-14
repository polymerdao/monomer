#!/bin/bash

set -e

SCRIPTS_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
MONOMER_DIR=$(cd "$SCRIPTS_DIR/.." && pwd)
GEN_DIR=$(cd "$MONOMER_DIR/gen" && pwd)
ROLLUP_DIR=$(cd "$MONOMER_DIR/x/rollup" && pwd)
TESTMODULE_DIR=$(cd "$MONOMER_DIR/testapp/x/testmodule" && pwd)

# generate cosmos proto code
buf generate

# move the generated rollup module message types to the x/rollup module
cp -r $GEN_DIR/rollup/v1/* $ROLLUP_DIR/types
rm -rf $GEN_DIR/rollup/v1

# move the generated testapp module message types to the testapp/x/testmodule module
cp -r $GEN_DIR/testapp/v1/* $TESTMODULE_DIR/types
rm -rf $GEN_DIR/testapp/v1

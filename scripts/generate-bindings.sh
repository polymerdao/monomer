#!/bin/bash

set -e

SCRIPTS_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
MONOMER_DIR=$(cd "$SCRIPTS_DIR/.." && pwd)
CONTRACTS_DIR=$(cd "$MONOMER_DIR/contracts" && pwd)
BINDINGS_DIR=$(cd "$MONOMER_DIR/bindings" && pwd)

# Create forge artifacts and cache directories if they don't exist
mkdir -p "$BINDINGS_DIR/artifacts"
mkdir -p "$BINDINGS_DIR/cache"
mkdir -p "$BINDINGS_DIR/generated"
FORGE_ARTIFACTS_DIR=$(cd "$BINDINGS_DIR/artifacts" && pwd)
FORGE_CACHE_DIR=$(cd "$BINDINGS_DIR/cache" && pwd)

# Check for jq, abigen, and forge installations
if ! command -v jq &> /dev/null
then
    echo "jq could not be found, please install jq for your system at https://jqlang.github.io/jq/download/"
    exit 1
fi
if ! command -v abigen &> /dev/null
then
    echo "abigen could not be found, please run 'make install-abigen'"
    exit 1
fi
if ! command -v forge &> /dev/null
then
    echo "forge could not be found, please install 'make install-foundry'"
    exit 1
fi

# Compile the contracts using forge
forge build --root . --contracts $CONTRACTS_DIR --out $FORGE_ARTIFACTS_DIR --cache-path $FORGE_CACHE_DIR

# Loop through each generated forge artifact
find $FORGE_ARTIFACTS_DIR -name "*.json" | while read -r forge_artifact; do
    # Extract contract name from the forge artifact
    contract_name=$(basename $forge_artifact .json)

    # Paths for the abi and bin files
    artifact_dir="${BINDINGS_DIR}/artifacts/${contract_name}.sol"
    abi_file="${artifact_dir}/${contract_name}.abi"
    bin_file="${artifact_dir}/${contract_name}.bin"

    # Extract the abi and bin from the forge artifact
    jq -r ".abi" $forge_artifact > $abi_file
    jq -r ".deployedBytecode.object" $forge_artifact > $bin_file

    # Generate Go bindings using abigen
    go_binding_file="${BINDINGS_DIR}/generated/${contract_name}.go"
    abigen --abi=$abi_file --bin=$bin_file --pkg="bindings" --type=$contract_name --out=$go_binding_file

    echo "Generated Go bindings for ${contract_name}.sol: ${go_binding_file}"
done

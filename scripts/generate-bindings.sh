#!/bin/bash

set -e

SCRIPTS_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
MONOMER_DIR=$(cd "$SCRIPTS_DIR/.." && pwd)
CONTRACTS_ROOT_DIR=$(cd "$MONOMER_DIR/contracts" && pwd)
CONTRACTS_DIR=$(cd "$CONTRACTS_ROOT_DIR/src" && pwd)
BINDINGS_DIR=$(cd "$MONOMER_DIR/bindings" && pwd)
OPTIMISM_CONTRACTS="$MONOMER_DIR/contracts/optimism-contracts.json"
OPTIMISM_CONTRACTS_DIR="$MONOMER_DIR/e2e/optimism/packages/contracts-bedrock/src"

# Create a directory to store temporary copies of the optimism contracts
mkdir -p "$CONTRACTS_ROOT_DIR/optimism/src"
TEMP_OP_CONTRACTS_ROOT_DIR=$(cd "$CONTRACTS_ROOT_DIR/optimism/" && pwd)
TEMP_OP_CONTRACTS_DIR=$(cd "$TEMP_OP_CONTRACTS_ROOT_DIR/src" && pwd)

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

# Check for the optimism-contracts.json file
if [ ! -f $OPTIMISM_CONTRACTS ]; then
    echo "optimism-contracts.json file not found"
    exit 1
fi

# Temporarily copy each required optimism contract to the monomer contracts directory
jq -c '.contracts + .deps | to_entries[]' $OPTIMISM_CONTRACTS | while read -r contract; do
    contract_file=$(echo $contract | jq -r '.value')
    contract_dir=$(dirname $contract_file)
    mkdir -p $TEMP_OP_CONTRACTS_DIR/$contract_dir
    cp $OPTIMISM_CONTRACTS_DIR/$contract_file $TEMP_OP_CONTRACTS_DIR/$contract_dir
done

# Compile the monomer contracts using forge
echo "Compiling monomer contracts..."
forge build --root $CONTRACTS_ROOT_DIR --contracts $CONTRACTS_DIR --out $FORGE_ARTIFACTS_DIR --cache-path $FORGE_CACHE_DIR

# Compile the optimism contracts using forge
echo "Compiling optimism contracts..."
forge build --root $TEMP_OP_CONTRACTS_ROOT_DIR --contracts $TEMP_OP_CONTRACTS_DIR --out $FORGE_ARTIFACTS_DIR --cache-path $FORGE_CACHE_DIR

# Remove the copied optimism contracts from the monomer contracts directory
rm -rf $TEMP_OP_CONTRACTS_ROOT_DIR

# Loop through each generated forge artifact to generate Go bindings
find $FORGE_ARTIFACTS_DIR -name "*.json" | while read -r forge_artifact; do
    # Extract contract name from the forge artifact
    contract_name=$(basename $(dirname $forge_artifact) .sol)

    # Do not generate bindings for contract dependencies
    if jq -e --arg key $contract_name '.deps[$key]' "$OPTIMISM_CONTRACTS" > /dev/null; then
        continue
    fi

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

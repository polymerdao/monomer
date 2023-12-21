#!/usr/bin/env bash
set -euo pipefail

if (( BASH_VERSINFO[0] < 4 )) ; then
  echo "Error: bash version 4+ required! Please update your bash installation." 1>&2
  exit 1
fi

go="${GO:-go}"
basedir="$( dirname "$( realpath "$0" )" )"

golist() {
  pushd "$basedir" > /dev/null
  "$go" list -f '{{ .Dir }}' -mod=readonly -m  "$1"
  popd > /dev/null
}

proto_dependencies=(
  github.com/cosmos/ibc-go/v7
  github.com/cosmos/cosmos-sdk
  github.com/cosmos/gogoproto
  github.com/cosmos/cosmos-proto
)

mapfile -t proto_imports < <(
  grep ^import -r "$basedir/proto" | cut -f2 -d' '| tr -d '"' | tr -d ';' | sort -u
)

# this is a transient dependency that won't show up above, so add it manually
proto_imports+=(
  'cosmos_proto/cosmos.proto'
)

# use an associative array to remove duplicated paths
declare -A dependency_map

# we need to find the full path of the directory where the proto buf is imported from.
# For example, in a protobuf file there's an import like this:
#   import "ibc/core/channel/v1/channel.proto";
# The trick is to find the full path P such that "$P/ibc/core/channel/v1/channel.proto" exists on disk
for proto_dep in "${proto_dependencies[@]}"; do
  for proto_import in "${proto_imports[@]}"; do
    proto_dep_full_path="$( golist "$proto_dep" )"
    proto_file="$( basename "$proto_import" )"

    while read -r proto_file_path; do
      # we got a hit in the form of "$P/foo/bar.proto" but we only care about P, so this code here
      # removes the 'foo/bar.proto' part of the path, leaving P.
      path_to_proto_dir="$( awk "{ gsub(\"$proto_import\", \"\", \$1); print \$1 }" <<< "$proto_file_path" )"
      dependency_map+=(["$path_to_proto_dir"]='')

    # this is where we look for protobuf files in the dependency directory and try to match
    # the path used within the import. If we get a hit, then we include the path
    done < <( find "$proto_dep_full_path" -name "$proto_file" | grep "$proto_import" )
  done
done

for d in "${!dependency_map[@]}"; do
  echo -n "-I $d "
done

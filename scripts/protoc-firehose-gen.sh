#!/usr/bin/env bash

set -eo pipefail

# create firehose files on an individual basis  w/ `buf generate`
proto_dirs=$(find ./proto/sf -path -prune -o -name '*.proto' -print0 | xargs -0 -n1 dirname | sort | uniq)
for dir in $proto_dirs; do
  proto_files=$(find "${dir}" -maxdepth 1 -name '*.proto')
  for file in $proto_files; do
    echo "Generating $file"
    buf generate --template proto/buf.gen.go.yaml "$file"
  done
done

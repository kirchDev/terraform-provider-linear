#!/usr/bin/env bash
# Fetch Linear's GraphQL schema (SDL) to .linear-schema.graphql.
#
# This is the field source of truth while building resources: which mutations
# exist, which inputs they take, which fields are nullable. Linear's own SDK
# repo generates it from the live API on every release, so this pulls that
# artifact rather than running an introspection query (which would need a token).
#
# The file is ~1.2 MB of generated SDL and gitignored — regenerate it, don't
# commit it. Handy lookups once it's there:
#
#   awk '/^input CustomViewCreateInput \{/,/^\}/' .linear-schema.graphql
#   awk '/^type Mutation \{/,/^\}/' .linear-schema.graphql | grep -oE '^  [a-zA-Z]+\('
#
# Requires: curl.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${REPO}/.linear-schema.graphql"
URL="https://raw.githubusercontent.com/linear/linear/master/packages/sdk/src/schema.graphql"

echo "==> fetching ${URL}"
curl -fsSL "$URL" -o "$OUT"

echo "==> wrote ${OUT} ($(wc -l < "$OUT") lines)"

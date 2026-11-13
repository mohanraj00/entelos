#!/bin/sh
# Make v030-store.json with the v0.3.0 Host. Run from the repository root:
#   sh src/lifecycle-core/testdata/migration/make_fixture.sh WORK
# WORK is a new directory outside the checkout.
set -eu
work=$1
here=src/lifecycle-core/testdata/migration
mkdir "$work"
commit=$(git rev-parse 'v0.3.0^{commit}')
git archive v0.3.0 src/lifecycle-core | tar -x -C "$work"
cp "$here/make_v030_store_test.go" "$work/src/lifecycle-core/"
(cd "$work/src/lifecycle-core" && V030_COMMIT=$commit V030_FIXTURE_OUT="$work/v030-store.json" \
  go test -count=1 -run '^TestMakeV030Store$' .)
cp "$work/v030-store.json" "$here/v030-store.json"

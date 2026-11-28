#!/bin/sh
# Make v080-store.json and v080-wire.json with the v0.8.0 code. Run from the
# repository root:
#   sh src/lifecycle-core/testdata/migration/make_v080_fixture.sh WORK
# WORK is a new directory outside the checkout.
set -eu
work=$1
here=src/lifecycle-core/testdata/migration
mkdir "$work"
commit=$(git rev-parse 'v0.8.0^{commit}')
git archive v0.8.0 src/lifecycle-core src/protocol | tar -x -C "$work"
cp "$here/make_v080_store_test.go" "$work/src/lifecycle-core/"
cp "$here/make_v080_wire_test.go" "$work/src/lifecycle-core/protocol/"
(cd "$work/src/lifecycle-core" && V080_COMMIT=$commit V080_FIXTURE_OUT="$work/v080-store.json" \
  go test -count=1 -run '^TestMakeV080Store$' .)
(cd "$work/src/lifecycle-core" && V080_COMMIT=$commit V080_STORE="$work/v080-store.json" V080_WIRE_OUT="$work/v080-wire.json" \
  go test -count=1 -run '^TestMakeV080Wire$' ./protocol)
cp "$work/v080-store.json" "$work/v080-wire.json" "$here/"

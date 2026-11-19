#!/bin/sh
# The seam test of #113 on a local kind cluster. Use: kind/seam.sh WORK
# WORK is a new directory outside the checkout. The script makes one kind
# cluster with its own kubeconfig in WORK, and it deletes the cluster at the
# end. It does not change the kubeconfig of the user.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
work=${1:?Give a new work directory outside the checkout.}
if [ -e "$work" ]; then
  echo "The work directory $work exists. Give a new directory." >&2
  exit 2
fi
mkdir -p "$work"
work=$(cd "$work" && pwd)
. "$here/pins.env"

for tool in docker kind kubectl helm go openssl; do
  command -v "$tool" >/dev/null || { echo "The seam test needs $tool." >&2; exit 2; }
done
case "$(kind version)" in
  "kind $KIND_VERSION "*) ;;
  *) echo "The seam test needs kind $KIND_VERSION, not: $(kind version)" >&2; exit 2 ;;
esac

cluster=entelos-agent-step
export KUBECONFIG="$work/kubeconfig"
cleanup() {
  status=$?
  if [ "${KEEP_CLUSTER:-0}" != 1 ]; then
    kind delete cluster --name "$cluster" --kubeconfig "$KUBECONFIG" >>"$work/kind.log" 2>&1 || true
  fi
  exit "$status"
}
trap cleanup EXIT INT TERM

# KIND_API_PORT fixes the host port of the API server. It is optional.
if [ -n "${KIND_API_PORT:-}" ]; then
  printf 'kind: Cluster\napiVersion: kind.x-k8s.io/v1alpha4\nnetworking:\n  apiServerAddress: 127.0.0.1\n  apiServerPort: %s\n' "$KIND_API_PORT" >"$work/kind.yaml"
  set -- --config "$work/kind.yaml"
else
  set --
fi
kind create cluster --name "$cluster" --image "$KIND_NODE_IMAGE" --kubeconfig "$KUBECONFIG" --wait 180s ${1+"$@"} >"$work/kind.log" 2>&1
kubectl wait --for=condition=Ready node --all --timeout=120s >>"$work/kind.log"

kubectl create namespace koodam-system >>"$work/install.log"
printf 'sk-%s' "$(openssl rand -hex 32)" | kubectl -n koodam-system create secret generic koodam-gateway --from-file=master-key=/dev/stdin >>"$work/install.log"
# The digest selects the chart. The version is in the name of the release note only.
helm install koodam "$KOODAM_CHART@$KOODAM_CHART_DIGEST" -n koodam-system -f "$here/values.yaml" --wait --timeout 10m >>"$work/install.log" 2>&1
helm list -n koodam-system -o json >"$work/helm-release.json"
grep -q "\"app_version\":\"$KOODAM_VERSION\"" "$work/helm-release.json" || { echo "The chart is not koodam $KOODAM_VERSION." >&2; exit 1; }
kubectl -n koodam-system rollout status deployment --timeout=5m >>"$work/install.log"
kubectl create namespace entelos-steps >>"$work/install.log"

cd "$here/.."
ENTELOS_KIND_KUBECONFIG="$KUBECONFIG" ENTELOS_KIND_NAMESPACE=entelos-steps ENTELOS_AGENT_IMAGE="$AGENT_IMAGE" \
  ENTELOS_KOODAM_STANDIN_296="$KOODAM_STANDIN_296" \
  go test -mod=readonly -count=1 -v -run '^TestKind' . 2>&1 | tee "$work/seam.log"
grep -q '^--- PASS: TestKindSeam ' "$work/seam.log" && grep -q '^--- PASS: TestKindFailedRun ' "$work/seam.log" &&
  grep -q '^--- PASS: TestKindBudgetStop ' "$work/seam.log" && grep -q '^ok ' "$work/seam.log" ||
  { echo "The seam test failed. The log is $work/seam.log." >&2; exit 1; }
kubectl -n entelos-steps get agents.core.koodam.dev -o json >"$work/agents.json"
echo "The seam test passed with koodam $KOODAM_VERSION (stand-in for koodam#296: $KOODAM_STANDIN_296). The log is $work/seam.log."

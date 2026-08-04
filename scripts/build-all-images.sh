#!/usr/bin/env bash
# build-all-images.sh — the repeatable fork-sanity gate (Plan 03-03, Task 3;
# D-16). This script has TWO jobs at once, and the second is the important
# one:
#   1. Produce every `athena-<service>` image Plan 03-06 deploys.
#   2. PROVE the hard fork (Plan 03-03, Task 1) is working code, not a
#      directory of files nobody has run. A fork that has never been
#      compiled is not a fork, it is a copy.
#
# Discovery is by the presence of a Dockerfile under src/<service>/ — this
# script carries no hardcoded per-service skip list (a skip list is exactly
# how a silently broken service would hide). The media service is the one
# documented, deliberate exception (D-16: media is CI-owned from Plan 03-07
# onward, and having two build paths for the same image is how tags
# diverge) — it is excluded by name, not omitted from discovery.
#
# The build context for each service is the DIRECTORY CONTAINING that
# service's own Dockerfile, not always the top-level src/<service>/
# directory — most vendored services keep their Dockerfile at
# src/<service>/Dockerfile (context == src/<service>/), but cartservice
# (a .NET project) keeps its own at src/cartservice/src/Dockerfile with
# COPY paths relative to that subdirectory. Discovering the Dockerfile
# per-service and deriving the context from its location, rather than
# assuming a fixed layout, handles this without a special case.
#
# Every image is tagged with the current git short SHA — an IMMUTABLE tag,
# never `latest` or any other mutable/rolling tag (D-22, estate-wide, not
# CI-only). Pushed to the local registry's HOST-SIDE push target
# (localhost:5000 — see athena-infra/scripts/registry-smoke.sh for why this
# is a different address from the in-cluster pull reference,
# athena-registry:5000, which Plan 03-06's chart values use instead).
#
# Fails loudly: if any service fails to build, this script reports that
# service by NAME and exits non-zero. It builds every OTHER service first
# (does not abort on the first failure) so one broken service never hides
# failures in others — same convention as this estate's verify-*.sh scripts.
#
# Usage:
#   scripts/build-all-images.sh            # build and push every service
#   scripts/build-all-images.sh <service>  # build and push one service only
#
# Safe to run twice: docker buildx build --push is idempotent against
# unchanged source — a second run against the same git SHA produces the
# same tag with the same content.

set -euo pipefail

# ---------------------------------------------------------------------------
# Output helpers
# ---------------------------------------------------------------------------
_c_green() { printf '\033[32m%s\033[0m\n' "$1"; }
_c_red()   { printf '\033[31m%s\033[0m\n' "$1"; }
info() { printf '[build-all-images] %s\n' "$1"; }
ok()   { _c_green "[build-all-images] OK: $1"; }
fail() { _c_red   "[build-all-images] FAIL: $1"; }

# ---------------------------------------------------------------------------
# Setup
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${SCRIPT_DIR}/.."
SRC_DIR="${REPO_ROOT}/src"

# Host-side push target — this script runs as a host process (the developer
# machine today, the self-hosted CI runner from Plan 03-07 onward), never a
# pod, so it always uses the host-published port, matching
# registry-smoke.sh's HOST_PUSH_TARGET convention exactly.
REGISTRY="localhost:5000"

GIT_SHA="$(git -C "${REPO_ROOT}" rev-parse --short HEAD)"

TARGET_SERVICE="${1:-}"

declare -a BUILT_IMAGES=()
declare -a FAILED_SERVICES=()

if [ ! -d "${SRC_DIR}" ]; then
  fail "src/ directory not found at ${SRC_DIR}"
  exit 1
fi

info "Building from git SHA ${GIT_SHA}, pushing to ${REGISTRY}"
if [ -n "${TARGET_SERVICE}" ]; then
  info "Selective build: only '${TARGET_SERVICE}'"
fi

# ---------------------------------------------------------------------------
# Discover and build. Iterates every directory under src/, excluding the
# media service by name (D-16) and any directory selective builds don't
# match — never a hardcoded list of services expected to exist.
# ---------------------------------------------------------------------------
for svc_dir in "${SRC_DIR}"/*/; do
  svc="$(basename "${svc_dir}")"

  # The one deliberate, documented exclusion — not a skip list of
  # arbitrarily broken services (D-16, UPSTREAM.md).
  if [ "${svc}" = "media" ]; then
    continue
  fi

  if [ -n "${TARGET_SERVICE}" ] && [ "${svc}" != "${TARGET_SERVICE}" ]; then
    continue
  fi

  dockerfile="$(find "${svc_dir}" -maxdepth 3 -iname 'Dockerfile' | head -1)"
  if [ -z "${dockerfile}" ]; then
    # Not every directory under src/ is necessarily a buildable service —
    # discovery by Dockerfile presence means a directory without one is
    # simply not in scope, never a silent build failure.
    info "Skipping ${svc}: no Dockerfile found under ${svc_dir}"
    continue
  fi

  context_dir="$(dirname "${dockerfile}")"
  image="${REGISTRY}/athena-${svc}:${GIT_SHA}"

  info "Building ${svc} -> ${image} (context: ${context_dir#"${REPO_ROOT}/"}, Dockerfile: ${dockerfile#"${REPO_ROOT}/"})"

  if docker buildx build --push -t "${image}" -f "${dockerfile}" "${context_dir}"; then
    ok "${svc} built and pushed: ${image}"
    BUILT_IMAGES+=("${image}")
  else
    fail "${svc} failed to build"
    FAILED_SERVICES+=("${svc}")
  fi
done

if [ -n "${TARGET_SERVICE}" ] && [ ${#BUILT_IMAGES[@]} -eq 0 ] && [ ${#FAILED_SERVICES[@]} -eq 0 ]; then
  fail "requested service '${TARGET_SERVICE}' not found under ${SRC_DIR} (or has no Dockerfile)"
  exit 1
fi

# ---------------------------------------------------------------------------
# Summary — printed even on failure, so Plan 03-06's chart values can read
# exactly which images landed rather than reconstructing references by
# convention.
# ---------------------------------------------------------------------------
echo
info "=== Build summary ==="
if [ ${#BUILT_IMAGES[@]} -gt 0 ]; then
  info "Built and pushed (${#BUILT_IMAGES[@]}):"
  for img in "${BUILT_IMAGES[@]}"; do
    printf '  %s\n' "${img}"
  done
fi

if [ ${#FAILED_SERVICES[@]} -gt 0 ]; then
  echo
  fail "Failed (${#FAILED_SERVICES[@]}): ${FAILED_SERVICES[*]}"
  exit 1
fi

echo
ok "All services built and pushed successfully — fork-sanity proven."

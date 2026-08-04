#!/usr/bin/env bash
# check-trivyignore.sh — D-23 auditable-exception lint for .trivyignore
# (Plan 03-07, Task 3), mirroring athena-infra's
# scripts/verify-checkov-skips.sh discipline for this estate's second
# scanner: every suppressed finding must carry a real, non-empty,
# non-boilerplate justification, or the lint fails the run.
#
# .trivyignore's own format has no built-in inline-justification syntax
# (unlike Checkov's `# checkov:skip=<ID>:<reason>` single-line form), so
# this estate's own convention (documented in .trivyignore's own header)
# is: the comment line(s) IMMEDIATELY ABOVE a finding-ID entry are that
# entry's justification. A finding ID with no comment directly above it,
# or a comment matching the boilerplate blocklist, fails this lint.
#
# This is the seed of the phase-7 estate-wide exception register — its
# shape matters more than its current emptiness (zero entries today is a
# legitimate, correct pass, not a hole in this lint).
#
# Deliberately not using `set -e` (house pattern, matches
# verify-checkov-skips.sh): one failing check must never abort the ones
# after it.
#
# Usage: bash scripts/check-trivyignore.sh

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${SCRIPT_DIR}/.."
IGNORE_FILE="${REPO_ROOT}/.trivyignore"

PASS_COUNT=0
FAIL_COUNT=0

check() {
  local name="$1" status="$2" observed="${3:-}"
  if [ "${status}" -eq 0 ]; then
    printf '\033[32mPASS\033[0m  %s\n' "${name}"
    PASS_COUNT=$((PASS_COUNT + 1))
  else
    printf '\033[31mFAIL\033[0m  %s\n' "${name}"
    printf '      observed: %s\n' "${observed}"
    FAIL_COUNT=$((FAIL_COUNT + 1))
  fi
}

# Same blocklist verify-checkov-skips.sh uses — a reason-less skip wearing
# a costume is rejected exactly like a genuinely empty one.
BOILERPLATE_REASONS=(
  "todo"
  "fixme"
  "n/a"
  "not applicable"
  "later"
  "see above"
  ""
)

is_boilerplate() {
  local candidate
  candidate="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | sed -E 's/^[[:space:]]*#*[[:space:]]*//; s/[[:space:].]+$//')"
  local bad
  for bad in "${BOILERPLATE_REASONS[@]}"; do
    if [ "${candidate}" = "${bad}" ]; then
      return 0
    fi
  done
  return 1
}

if [ ! -f "${IGNORE_FILE}" ]; then
  check ".trivyignore exists at the repo root" 1 "file not found: ${IGNORE_FILE}"
  echo
  printf '[check-trivyignore] %s passed, %s failed, %s total.\n' "${PASS_COUNT}" "$((FAIL_COUNT + 1))" "$((PASS_COUNT + FAIL_COUNT + 1))"
  exit 1
fi

# Finding-ID lines: non-blank, not a comment. Trivy's own ignore-file
# entries are bare identifiers (CVE-YYYY-NNNNN, GHSA-..., or a Trivy
# secret/misconfig rule ID) — never starting with '#' and never empty
# after trimming.
LAST_COMMENT=""
ENTRY_COUNT=0

while IFS= read -r RAW_LINE || [ -n "${RAW_LINE}" ]; do
  LINE="$(printf '%s' "${RAW_LINE}" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//')"

  if [ -z "${LINE}" ]; then
    # A blank line resets the "immediately above" adjacency — a
    # justification must sit directly above its entry, not floating
    # somewhere earlier in the file separated by whitespace.
    LAST_COMMENT=""
    continue
  fi

  if [[ "${LINE}" == \#* ]]; then
    LAST_COMMENT="${LINE}"
    continue
  fi

  # A genuine finding-ID entry.
  ENTRY_COUNT=$((ENTRY_COUNT + 1))
  if [ -z "${LAST_COMMENT}" ]; then
    check "entry '${LINE}' has a justification comment directly above it" 1 \
      "no comment line immediately precedes this entry"
  elif is_boilerplate "${LAST_COMMENT}"; then
    check "entry '${LINE}' has a real, non-boilerplate justification" 1 \
      "comment=[${LAST_COMMENT}] is empty or matches the boilerplate blocklist (${BOILERPLATE_REASONS[*]})"
  else
    check "entry '${LINE}' has a real, non-boilerplate justification" 0
  fi

  # A justification is consumed by exactly the entry directly below it —
  # two consecutive entries under one comment block must each restate
  # their own reason, not silently inherit the previous entry's.
  LAST_COMMENT=""
done <"${IGNORE_FILE}"

if [ "${ENTRY_COUNT}" -eq 0 ]; then
  # Zero suppressions is a legitimate, correct state (nothing to audit),
  # not a hole in this lint — matches verify-checkov-skips.sh's own
  # vacuous-pass convention.
  check "no .trivyignore entries found (vacuous pass is correct here: zero suppressions means zero exceptions to audit)" 0
fi

echo
printf '[check-trivyignore] %s passed, %s failed, %s total.\n' \
  "${PASS_COUNT}" "${FAIL_COUNT}" "$((PASS_COUNT + FAIL_COUNT))"

if [ "${FAIL_COUNT}" -gt 0 ]; then
  exit 1
fi
exit 0

# 0005. Image Scanning Threshold and the Exception Register

* Status: accepted
* Date: 2026-08-04
* Deciders: Yahia Tarek (YahiaEng)
* Tier: full-madr

## Context and Problem Statement

POL-03 requires image scanning; a gate needs a threshold. A gate that blocks
on everything gets bypassed; a gate that blocks on nothing is decoration.
Findings also need somewhere visible to live, and exceptions need auditable
justification.

## Considered Options

* **One blocking invocation covering all severities** — rejected: blocks on
  unfixable findings (gate-bypass culture) and the action's severity filter
  interacts unreliably with the reporting format (open upstream issues).
* **Report-only scanning, no gate** — rejected: POL-03 requires a gate, and
  findings without enforcement decay into wallpaper.
* **Dual invocation: report everything, block on fixable HIGH/CRITICAL,
  justification-linted exception file** (chosen).

## Decision

Trivy runs twice in one job (DB cached between invocations): a non-blocking
all-severities SARIF invocation feeding the repo's Code Scanning surface
(D-27), and a blocking table invocation restricted to HIGH/CRITICAL with
--ignore-unfixed (D-23) — the double invocation exists because
trivy-action's severity filter interacts unreliably with the reporting
format (upstream issues #435/#309, confirmed open at execution). ignore-
unfixed is deliberate: blocking on findings nobody can act on teaches
people to bypass the gate, and a bypassed gate is worse than none.
.trivyignore is the exception register: every entry must carry a
justification comment naming the finding and the acceptance reason, linted
by scripts/check-trivyignore.sh as a workflow step (unjustified entry =
red run). This is the seed of Phase 7's estate-wide register.

## Consequences

* The gate's threshold is explainable in one sentence and its exceptions
  are self-documenting.
* The publish job depends on scan success — nothing the gate rejected can
  reach the registry, keeping the scan verdict attached to the artefact.

# 0001. Merge Queue on the App Monorepo Only

* Status: accepted
* Date: 2026-08-03
* Deciders: Yahia Tarek (YahiaEng)
* Tier: short-form

## Context

`athena-app`, `athena-infra`, and `athena-gitops` each have different merge
concurrency needs. `athena-app` is a monorepo with path-filtered CI where
multiple service PRs can legitimately merge close together and need a
queue that verifies the *combined* result before any of them lands, not
just each PR in isolation. `athena-gitops` needs its bot-authored
promotion commits to land fast, with no queueing latency in the path.
`athena-infra`'s Terraform applies serialize at a different layer
entirely.

## Decision

A native GitHub merge queue (`grouping_strategy = "ALLGREEN"`) is
configured on `athena-app`'s `main` ruleset only, via `governance/
protections.tf`. Every `athena-app` workflow — including the current
placeholder `lint.yml` — carries an `on: merge_group` trigger from day
one, deliberately ahead of when it's strictly needed, because retrofitting
that trigger onto every existing workflow later (once Phase 3 lands the
real CI) would be a wider, riskier change than adding it now while the
workflows are still simple.

`athena-gitops` stays queue-free: its bot commit path (Phase 3's image-
promotion writes to `envs/<env>`) must stay fast, and a merge queue would
add latency to a path that has no multi-PR-combination risk to protect
against. `athena-infra` also stays queue-free — its own serialization need
(preventing concurrent `terraform apply` runs against the same state) is
solved at the correct layer by Actions concurrency groups (`terraform-
<env>`, `cancel-in-progress: false`), a mechanism suited to protecting a
single shared resource rather than combining independent PRs.

## Consequences

* `athena-app` PRs merge only after the queue's own CI run against the
  speculatively-combined result passes — catching interaction bugs between
  concurrently-merging service changes that per-PR-isolated checks would
  miss.
* Every future `athena-app` workflow must include the `merge_group` trigger
  from the moment it's created, or it will silently not participate in the
  queue's verification — a convention worth re-checking whenever a new
  workflow file is added to this repo.
* `athena-gitops` and `athena-infra` PRs merge as soon as their own
  required checks and reviews pass, with no queue-induced latency —
  matching each repo's actual concurrency risk rather than applying one
  policy uniformly across all three delivery repos.

# athena-app

The **Athena** service monorepo — the workload half of the Athena estate
(interview-prep project — see the platform handbook, `athena-docs`, for the
full estate-level story). This repo will hold:

- A rebranded fork of Google's Online Boutique
  ([GoogleCloudPlatform/microservices-demo](https://github.com/GoogleCloudPlatform/microservices-demo)),
  11 gRPC/polyglot microservices, arriving in Phase 3.
- One custom Go **media** service (Postgres with migrations, S3 uploads
  behind CloudFront, Redis/Valkey-backed sessions) — deliberately bare
  observability (no metrics endpoint, basic logs only) as an instrumentation
  practice target for Phase 4.

## Why a monorepo (not repo-per-service)

The estate deliberately keeps every service in one repository instead of the
naive one-repo-per-microservice split. Two reasons drive this, both
interview-relevant:

- **Path-filtered CI is the harder, more valuable problem to solve.**
  Change-detection-driven selective builds/tests/publishes (only the services
  that actually changed run through CI) is a real production pattern at
  companies running dozens of services from one repo — a polyrepo split would
  let CI skip this problem entirely rather than teach it.
- **Shared tooling with no duplicated boilerplate.** One `.github/workflows/`
  set, one dependency-update story, one place a cross-service change (e.g. a
  shared proto or a common library bump) lands atomically — twelve
  near-identical repos would multiply the same CI/lint/scan config twelve
  times for no operational benefit at this project's scale.

Repo-per-service was considered and rejected; the full trade-off (concurrency
queues, CODEOWNERS-per-path instead of CODEOWNERS-per-repo, blast radius of a
bad change) is recorded as an ADR in the platform handbook (`athena-docs`).

## Status

This repository is currently a skeleton — `docs/adr/` is the only structure
Plan 04 seeds here. Phase 2's CI walking-skeleton and Phase 3's app code are
the next things to land. `.github/` is deliberately **not** created by this
plan — Plan 05 owns CODEOWNERS and the seed lint/validate workflows.

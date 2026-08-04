# 0003. Media Service Scope and Deliberate Uninstrumentation

* Status: accepted
* Date: 2026-08-04
* Deciders: Yahia Tarek (YahiaEng)
* Tier: full-madr

## Context and Problem Statement

The estate needs one service it owns end to end — schema, sessions, object
storage, CI — as the fixture every infrastructure phase exercises. What does
it deliberately include, and why does it ship with almost no observability
instrumentation in Phase 3?

## Decision

The media service is a small Go service with exactly the state that makes
infrastructure problems real: golang-migrate-managed Postgres schema
(including a deliberately-reversible 4-step chain CI runs up→down→up),
Valkey-backed bearer sessions, and S3 uploads with magic-byte validation.
It ships in Phase 3 with only /healthz and default process metrics —
deliberately uninstrumented: Phase 4's observability work needs a service
whose instrumentation IS the exercise, not one that arrives pre-solved.
Startup-cached config (bucket name, addresses read once at boot) is likewise
deliberate: it manufactures the config-staleness/rotation problem Phase 5's
Vault work exists to solve.

## Consequences

* Each later phase has a real, owned surface to retrofit (metrics, tracing,
  secrets rotation) instead of a demo app that hides the work.
* The service doubles as the CI fixture: ephemeral-Postgres migration gate,
  scan gate, publish/handoff — all proven against code this estate owns.

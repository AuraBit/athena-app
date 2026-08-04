# 0004. Registry-Backed BuildKit Cache on Self-Hosted Runners

* Status: accepted
* Date: 2026-08-04
* Deciders: Yahia Tarek (YahiaEng)
* Tier: madr-lite

## Context and Problem Statement

Media's multi-stage Go build spends its time in `go mod download` and
`go build` inside the discarded builder stage. Where should BuildKit's
cache live so CI rebuilds are fast, given self-hosted runners whose local
state must be treated as disposable?

## Decision

`--cache-to type=registry,ref=localhost:5000/athena-media-cache,mode=max`
and `--cache-from` the same ref. mode=max exports intermediate-stage
layers (min exports only the final image's — near-worthless for multi-stage
Go). The cache repository is deliberately distinct from the release
repository so a cache layer can never be mistaken for a release artefact.
Measured live: cold build ~49s; cleared-local-cache build importing only the
registry cache ~2s.

## Consequences

* Runner reprovisioning (ephemeral/JIT model, ADR-0005 in athena-infra)
  costs no build-cache warmup.
* GitHub's `type=gha` cache backend was rejected: it is scoped to
  GitHub-hosted infrastructure and ~10GB eviction, while the local registry
  is already the estate's artifact plane.

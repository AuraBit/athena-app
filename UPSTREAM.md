# Upstream provenance

`src/adservice`, `src/cartservice`, `src/checkoutservice`, `src/currencyservice`,
`src/emailservice`, `src/frontend`, `src/loadgenerator`, `src/paymentservice`,
`src/productcatalogservice`, `src/recommendationservice`, and `src/shippingservice`
are a hard fork of Google's Online Boutique demo application.

- **Upstream repository:** [`GoogleCloudPlatform/microservices-demo`](https://github.com/GoogleCloudPlatform/microservices-demo)
- **Release tag snapshotted:** `v0.10.6`
- **Commit SHA:** `5b3a712ab85ccb8f6f7cd5b720d36ba9a8d041eb`
- **Snapshot date:** 2026-08-04
- **License:** [Apache License, Version 2.0](https://www.apache.org/licenses/LICENSE-2.0). Every
  vendored source file's original upstream copyright/license header (typically
  `Copyright 20xx Google LLC`, `Licensed under the Apache License, Version 2.0`) is preserved
  unmodified — this rebrand changes displayed product identity only, never provenance metadata.
  This repository's own `LICENSE` (MIT) governs Athena's own code (`src/media`, the CI/CD
  tooling, docs); the vendored directories above remain Apache-2.0 licensed as their own headers
  state.

## This is a clean-break hard fork — never re-synced

This fork is a one-time file copy, not a git operation: no upstream remote was ever added, no
submodule, no subtree. `git remote -v` in this repository lists only this estate's own `origin`.

**This fork will never be re-synced from upstream.** The destination for `src/frontend` and its
siblings is a full rebrand (see the deferred rename below) — carrying an upstream tracking
relationship forward would mean every future upstream change has to be evaluated against a
codebase that is deliberately diverging from it, a permanent conflict tax with no offsetting
benefit for a $0 study project. Divergence from this point forward is free precisely because there
is nothing left to reconcile against.

## Exclusions (D-15 amendment)

D-15 originally read "all Boutique services plus loadgenerator deploy — full-estate fidelity."
Live research against the upstream repository at execution time found upstream itself ships
twelve directories under `src/` but only eleven services in its own deployable base manifests —
`shoppingassistantservice` is excluded there too, because it depends on Google-managed AlloyDB and
Vertex AI, neither of which has a local, $0 stand-in.

**Decision (checkpoint D-15, pre-answered by the project owner): exclude
`shoppingassistantservice`, treated code-and-docs-only — the same treatment this estate already
gives Karpenter.** It is not vendored into `src/` at all; there is no `src/shoppingassistantservice`
directory in this repository, and `scripts/build-all-images.sh` has no image to build for it because
no directory means no Dockerfile is discovered.

This is recorded here, explicitly and by name, rather than left as an unremarked gap: eleven
services deploy, matching upstream's own base deploy and every "eleven microservices" reference
already in this estate's docs, and nothing in Phases 4 through 7 depends on the twelfth.

**Rejected alternatives** (recorded per this plan's own prohibition against silently narrowing
scope):
- *Include it with a local substitute for AlloyDB and the AI backend* — rejected: standing up a
  Postgres-compatible AlloyDB substitute and a local model/stub for the AI calls is net-new scope
  with no requirement behind it, and risks blocking the storefront demo on an unrelated
  substitution problem.
- *Vendor its source but never deploy it* — rejected: shipping a service that has never been built
  or run would force `scripts/build-all-images.sh`'s fork-sanity gate to either skip it (weakening
  the gate that exists specifically to prove vendored code compiles) or fail permanently.

## Deferred: the full module-path and package rename

Task 2 of this plan (03-03) rebrands only the *visible* surface — the storefront's displayed
product name, page titles, and served logo asset become Athena. It deliberately does **not**
rename any Go module path, package name, import path, internal type name, protobuf package
identifier, or service directory name — those all stay exactly as upstream left them.

This is recorded here as an explicit outstanding item, not an oversight: the point of the clean
break above is that this full rename can happen later at zero cost (no upstream tracking to
reconcile against), and a half-done rename now (some identifiers renamed, others not, mid-fork)
would be strictly worse than either finishing it fully or leaving it fully deferred. No phase in
this project's current roadmap currently claims this item; it is future work.

## Reference, not vendored deployable YAML

Upstream's own Kubernetes manifests (`kubernetes-manifests/`, `helm-chart/`) are **not** vendored
into this repository. They exist only as a reference spec of ports, environment variables, and
inter-service dependencies, consulted when authoring this estate's own Helm chart
(`athena-gitops/charts/athena`, Plan 03-06). See
[`docs/upstream-reference/README.md`](docs/upstream-reference/README.md) for the specific
constraint that reference informs.

<p align="center">
  <img src="docs/assets/athena-logo.svg" alt="Athena logo" width="130">
</p>

<h1 align="center">athena-app</h1>

<p align="center">
  The service monorepo of the <a href="https://github.com/AuraBit">Athena estate</a> — the workload half of a production-grade DevOps platform that runs entirely on a laptop, for $0.
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
  <img src="https://img.shields.io/badge/CI-merge%20queue%20%2B%20CODEOWNERS-2088FF?logo=githubactions&logoColor=white" alt="CI: merge queue + CODEOWNERS">
  <img src="https://img.shields.io/badge/status-workloads%20incoming-8957e5" alt="Status: workloads incoming">
</p>

---

Athena replicates the CI/CD pipeline and infrastructure estate that large engineering organisations run in production, built in public so it can be studied, broken, fixed, and explained. This repo holds the application workloads. The platform story — clusters, DNS, TLS, AWS emulation, governance — lives in [`athena-infra`](https://github.com/AuraBit/athena-infra); the handbook with all estate-wide decisions is [`athena-docs`](https://github.com/AuraBit/athena-docs).

Two workloads land here:

- A rebranded fork of Google's
  [Online Boutique](https://github.com/GoogleCloudPlatform/microservices-demo) —
  11 gRPC microservices across several languages, adapted into this estate's
  Helm + Kustomize layout.
- One custom Go **media** service: Postgres with real migrations, S3 uploads
  behind CloudFront, Valkey-backed sessions. It ships with deliberately bare
  observability (no metrics endpoint, basic logs only) because instrumenting
  an under-observed service properly is one of the estate's practice targets.

## Why a monorepo (not repo-per-service)

The estate deliberately keeps every service in one repository instead of the
naive one-repo-per-microservice split. Two reasons drive this, and both are
the kind of thing you should be able to defend in a design review:

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
bad change) is recorded in the ADRs — see
[this repo's ADR-0001](docs/adr/0001-merge-queue-on-the-app-monorepo-only.md)
on why the merge queue exists only here, and the topology ADR in
[`athena-docs`](https://github.com/AuraBit/athena-docs).

## What's here today

The governance layer landed first, on purpose: `CODEOWNERS` path routing, a
lint workflow wired into branch protection, and a merge queue on `main`.
The pipeline was proven against a skeleton before any real service code, then
the media service's ArgoCD-synced tracer went live, and now the Boutique fork
below has landed too.

### Service inventory

| Service | Language | Owning team |
|---|---|---|
| `src/frontend` | Go | team-storefront |
| `src/adservice` | Java | team-storefront |
| `src/recommendationservice` | Python | team-storefront |
| `src/cartservice` | C# | team-commerce |
| `src/checkoutservice` | Go | team-commerce |
| `src/paymentservice` | JavaScript | team-commerce |
| `src/shippingservice` | Go | team-commerce |
| `src/productcatalogservice` | Go | team-catalog |
| `src/currencyservice` | JavaScript | team-catalog |
| `src/emailservice` | Python | team-comms |
| `src/loadgenerator` | Python (Locust) | team-comms |
| `src/media` | Go (custom) | team-media |

Eleven of the twelve upstream Online Boutique services are vendored and deployed;
`shoppingassistantservice` is excluded (depends on managed AlloyDB + Vertex AI with
no $0 local stand-in) — see [`UPSTREAM.md`](UPSTREAM.md)'s exclusions section.

### Building

Every vendored Boutique service builds and pushes as an immutably-tagged
`athena-<service>` image to the local registry via:

```bash
scripts/build-all-images.sh          # every service
scripts/build-all-images.sh frontend # a single service, for fast iteration
```

The media service builds its own Dockerfile directly (`src/media/Dockerfile`) — it
is CI-owned from Plan 03-07 onward, not part of `build-all-images.sh`'s scope.

## Provenance

The Online Boutique fork under `src/` (everything except `src/media`) is vendored from
[GoogleCloudPlatform/microservices-demo](https://github.com/GoogleCloudPlatform/microservices-demo)
at a pinned release tag, Apache-2.0 licensed. Full provenance, the pinned tag/commit, the
no-resync declaration, and the one deliberate exclusion are recorded in [`UPSTREAM.md`](UPSTREAM.md).

## License

[MIT](LICENSE)

# Upstream reference (not vendored deployable YAML)

Upstream's own Kubernetes manifests
([`kubernetes-manifests/`](https://github.com/GoogleCloudPlatform/microservices-demo/tree/v0.10.6/kubernetes-manifests))
and Helm chart
([`helm-chart/`](https://github.com/GoogleCloudPlatform/microservices-demo/tree/v0.10.6/helm-chart))
are deliberately **not** copied into this repository as deployable YAML. This estate authors its
own Helm chart + Kustomize overlays (`athena-gitops/charts/athena`, D-17/D-28/D-29) for every
deployed unit, media included — upstream's manifests exist here only as a **reference spec**: the
ports each service listens on, the environment variables each service expects, and which services
depend on which, consulted while writing that chart (Plan 03-06).

## Pitfall 4: the cart service's cache address is a literal, not a Helm value

**This is the single easiest silent failure in the whole storefront deploy — read this before
authoring `athena-gitops`'s cartservice template.**

Verified live against `kubernetes-manifests/cartservice.yaml` at tag `v0.10.6`: cartservice's
Deployment hard-codes its Redis connection address as a literal environment variable value, not a
Helm-templated one:

```yaml
env:
- name: REDIS_ADDR
  value: "redis-cart:6379"
```

`redis-cart` is a literal Kubernetes Service DNS name baked directly into the raw manifest.
Upstream's own Helm chart may parameterize this elsewhere, but the *manifest* — which is what this
estate's fork actually ships and builds from (D-13's clean-break hard fork copies source and
manifests-as-reference only, it does not adopt upstream's Helm templating layer) — does not.

**This estate's chart resolution (recorded authoritatively in
`athena-gitops/charts/athena/values.yaml`'s `datastores.valkey.cart` block and
`datastores-valkey.yaml`'s header comment, Plan 03-04 Task 3):** keep the cart Valkey instance's
Kubernetes Service named exactly `redis-cart`, even though the pod behind it runs Valkey, not
Redis (option (a) from the two documented alternatives below — the simpler of the two, matching
D-18's "replace in place" framing). Plan 03-06, when it vendors and templates `cartservice`, must
**not** rename this Service and must **not** need to override `REDIS_ADDR` — the existing Service
name already satisfies cartservice's hard-coded expectation.

The two alternatives, for the record:
1. **(chosen)** Keep the Service named `redis-cart`; the underlying pod is Valkey, wire-compatible
   with Redis, so cartservice's hard-coded `REDIS_ADDR=redis-cart:6379` resolves correctly with no
   code or manifest change to cartservice itself.
2. Rename the Service (e.g. to `athena-cart-cache`) and explicitly set `REDIS_ADDR` in
   cartservice's own chart template to override the vendored default.

**Warning signs if this constraint is violated:** cart contents don't persist across page loads,
or `checkoutservice` can't read the cart at checkout time; check `cartservice`'s pod logs for a
connection-refused error against `redis-cart:6379` specifically — that exact hostname appearing in
an error is the signature of this exact pitfall.

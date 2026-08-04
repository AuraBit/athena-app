# 0002. Hard Fork, Clean Break, of the Upstream Storefront

* Status: accepted
* Date: 2026-08-04
* Deciders: Yahia Tarek (YahiaEng)
* Tier: full-madr

## Context and Problem Statement

Athena needs a realistic multi-service app (APP-01). Online Boutique
(GoogleCloudPlatform/microservices-demo v0.10.6) provides 11 runnable
polyglot gRPC services — but how it is imported decides every later cost:
GitHub fork? git subtree/submodule with upstream tracking? or a vendored
snapshot with no upstream link?

## Considered Options

1. **GitHub fork / upstream remote** — rejected: keeps upstream's identity
   and history authoritative, makes the eventual full rebrand a permanent
   merge conflict, and implies an update contract this estate does not want.
2. **Submodule/subtree** — rejected: submodules break worktree/CI ergonomics
   and subtree merges reintroduce the tracking we are avoiding.
3. **Hard fork: clean file snapshot at a pinned tag** (chosen) — vendored
   into src/ with provenance recorded in UPSTREAM.md.

## Decision

Vendor the 11 in-scope services from the pinned upstream release tag as a
plain file copy; record tag + commit + exclusions in UPSTREAM.md; prove the
break by building every image from this repo's own source
(scripts/build-all-images.sh, 11/11). Per the D-15 amendment,
shoppingassistantservice is excluded: it requires a paid Google Gemini API
key and cannot run under the estate's strict-$0 constraint — an explicit,
recorded exception to "all Boutique services deploy."

## Consequences

* Rebrand and refactor cost nothing upstream-related, ever.
* Upstream CVE fixes arrive only by deliberate re-vendor (Phase 7's scanning
  suite owns detection) — a traded-away benefit, stated here honestly.
* UPSTREAM.md is the single provenance record interviews can point at.

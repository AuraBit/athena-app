# Dockerfile — multi-stage, distroless static, non-root (D-10, D-25).
#
# BuildKit cache mounts (`RUN --mount=type=cache`) persist the Go module and
# build caches ACROSS builds without baking them into any image layer — the
# `# syntax=` directive on line 1 is what makes BuildKit (not the classic
# builder) parse this file and honour those mounts.
#
# Final stage is distroless static, nonroot: no shell, no package manager, no
# libc even (CGO_ENABLED=0, statically linked) — `docker run --entrypoint
# /bin/sh` against the built image fails outright because there is no /bin/sh
# to exec. This is the interview-relevant "no-shell debugging" story D-10
# calls out: an attacker who gets code execution inside this container has no
# shell to pivot with, and normal operational debugging uses an ephemeral
# debug container (`kubectl debug`) instead of `kubectl exec`.
#
# D-25's registry cache (`--cache-to/--cache-from type=registry`) is applied
# at `docker buildx build` invocation time, not inside this file — see
# scripts/apply-coredns-custom.sh's sibling in athena-gitops/argocd/bootstrap
# for the exact command this task records in its commit message.
# syntax=docker/dockerfile:1
FROM golang:1.26 AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /out/media ./cmd/media

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/media /media
USER nonroot:nonroot
ENTRYPOINT ["/media"]

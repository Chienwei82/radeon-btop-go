# gputop-go development and release images.
#
# The host machine intentionally has NO Go toolchain. Everything (build, test,
# vet, lint) runs in the `dev` stage via scripts/podev.sh. The `release` stage
# produces the shippable static binary from a clean checkout with podman only.
#
# Pin rationale: patch releases of the Go toolchain arrive monthly; pinning the
# exact patch keeps every developer, CI runner and release byte-comparable.
# Bump deliberately via: edit tag below, rebuild dev image, record in
# memory-bank/progress.md ("Evolution of project decisions").

ARG GO_VERSION=1.27.2

# ---------------------------------------------------------------------------
# dev: full toolchain + golangci-lint, used by scripts/podev.sh
# ---------------------------------------------------------------------------
FROM docker.io/library/golang:${GO_VERSION}-bookworm AS dev

ARG GOLANGCI_LINT_VERSION=v2.14.0

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        git \
    && rm -rf /var/lib/apt/lists/* \
    && GOBIN=/usr/local/bin go install \
        github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION} \
    && golangci-lint version

ENV CGO_ENABLED=0 \
    GOFLAGS=-mod=readonly

WORKDIR /work

# ---------------------------------------------------------------------------
# release: static binary, no toolchain in the final image
# ---------------------------------------------------------------------------
FROM docker.io/library/golang:${GO_VERSION}-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gputop ./cmd/gputop

FROM gcr.io/distroless/static-debian12:nonroot AS release
COPY --from=build /out/gputop /gputop
ENTRYPOINT ["/gputop"]

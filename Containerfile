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
ARG VERSION=1.0.0

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

ARG VERSION

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# -buildvcs=false keeps a source archive without .git reproducible; the
# version is stamped rather than scraped so the binary, the man page and the
# package recipe can be checked against each other.
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/gputop ./cmd/gputop

FROM gcr.io/distroless/static-debian12:nonroot AS release

ARG VERSION

LABEL org.opencontainers.image.title="gputop" \
      org.opencontainers.image.description="btop-style terminal monitor for AMD Radeon GPUs" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.licenses="GPL-3.0-or-later" \
      org.opencontainers.image.source="https://github.com/gputop/gputop-go"

COPY --from=build /out/gputop /gputop
ENTRYPOINT ["/gputop"]

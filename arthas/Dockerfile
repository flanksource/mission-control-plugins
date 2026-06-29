ARG GO_VERSION=1.26.1
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm@sha256:ab3d6955bbc813a0f3fdf220c1d817dd89c0b3f283777db8ece4a32fe7858edd AS builder

ARG PLUGIN=arthas
ARG VERSION=dev
ARG BUILD_DATE
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG BUILDARCH
ARG NODE_VERSION=24.11.1
ARG TASK_VERSION=v3.45.4

WORKDIR /src
RUN set -eux; \
  apt-get update; \
  apt-get install -y --no-install-recommends ca-certificates curl xz-utils; \
  rm -rf /var/lib/apt/lists/*; \
  case "${BUILDARCH}" in amd64) node_arch=x64 ;; arm64) node_arch=arm64 ;; *) echo "unsupported build arch: ${BUILDARCH}" >&2; exit 1 ;; esac; \
  curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${node_arch}.tar.xz" -o /tmp/node.tar.xz; \
  tar -xJf /tmp/node.tar.xz -C /usr/local --strip-components=1; \
  rm /tmp/node.tar.xz; \
  corepack enable; \
  corepack prepare pnpm@10.33.0 --activate; \
  go install github.com/go-task/task/v3/cmd/task@${TASK_VERSION}

COPY Taskfile.yml ./
COPY ${PLUGIN}/ ./${PLUGIN}/
RUN MISSION_CONTROL_PLUGIN_PATH=/out \
  task build:plugin:ci PLUGIN=${PLUGIN} VERSION=${VERSION} BUILD_DATE="${BUILD_DATE}" TARGET_GOOS=${TARGETOS} TARGET_GOARCH=${TARGETARCH}

FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639
ARG PLUGIN=arthas
COPY --from=builder /out/${PLUGIN} /usr/local/bin/${PLUGIN}
EXPOSE 9000
ENTRYPOINT ["/usr/local/bin/arthas"]
CMD ["--serve", ":9000"]

ARG GO_VERSION=1.26.1
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm@sha256:ab3d6955bbc813a0f3fdf220c1d817dd89c0b3f283777db8ece4a32fe7858edd AS builder

ARG PLUGIN=kubernetes-metrics
ARG VERSION=dev
ARG BUILD_DATE
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG TASK_VERSION=v3.47.0

WORKDIR /src
RUN go install github.com/go-task/task/v3/cmd/task@${TASK_VERSION}
COPY Taskfile.yml ./
COPY ${PLUGIN}/ ./${PLUGIN}/
RUN MISSION_CONTROL_PLUGIN_PATH=/out \
  task build:plugin:ci PLUGIN=${PLUGIN} VERSION=${VERSION} BUILD_DATE="${BUILD_DATE}" TARGET_GOOS=${TARGETOS} TARGET_GOARCH=${TARGETARCH}

FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639
ARG PLUGIN=kubernetes-metrics
COPY --from=builder /out/${PLUGIN} /usr/local/bin/${PLUGIN}
EXPOSE 9000
ENTRYPOINT ["/usr/local/bin/kubernetes-metrics"]
CMD ["--serve", ":9000"]

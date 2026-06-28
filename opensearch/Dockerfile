ARG GO_VERSION=1.26.1
FROM golang:${GO_VERSION}-alpine AS builder

ARG PLUGIN=opensearch
ARG VERSION=dev
ARG BUILD_DATE
ARG TARGETOS=linux
ARG TARGETARCH=amd64
WORKDIR /src
COPY ${PLUGIN}/go.mod ${PLUGIN}/go.sum ./
RUN go mod download
COPY ${PLUGIN}/ ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w -X main.Version=${VERSION} -X 'main.BuildDate=${BUILD_DATE}'" -o /out/plugin .

FROM alpine:3.22
RUN apk --no-cache add ca-certificates && adduser -D -H -u 65532 plugin
COPY --from=builder /out/plugin /usr/local/bin/plugin
USER 65532:65532
EXPOSE 9000
ENTRYPOINT ["/usr/local/bin/plugin"]
CMD ["--serve", ":9000"]

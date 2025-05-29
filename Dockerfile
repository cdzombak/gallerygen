ARG BIN_NAME=gallerygen
ARG BIN_VERSION=<unknown>

FROM golang:1-alpine AS builder
ARG BIN_NAME
ARG BIN_VERSION
WORKDIR /src/gallerygen
COPY . .
RUN go build -ldflags="-X main.version=${BIN_VERSION}" -o ./out/${BIN_NAME} .

FROM scratch
ARG BIN_NAME
COPY --from=builder /src/gallerygen/out/${BIN_NAME} /usr/bin/gallerygen
ENTRYPOINT ["/usr/bin/gallerygen"]

LABEL license="LGPL3"
LABEL maintainer="Chris Dzombak <https://www.dzombak.com>"
LABEL org.opencontainers.image.authors="Chris Dzombak <https://www.dzombak.com>"
LABEL org.opencontainers.image.url="https://github.com/cdzombak/gallerygen"
LABEL org.opencontainers.image.documentation="https://github.com/cdzombak/gallerygen/blob/main/README.md"
LABEL org.opencontainers.image.source="https://github.com/cdzombak/gallerygen.git"
LABEL org.opencontainers.image.version="${BIN_VERSION}"
LABEL org.opencontainers.image.licenses="LGPL3"
LABEL org.opencontainers.image.title="${BIN_NAME}"
LABEL org.opencontainers.image.description="Generate a static HTML gallery from a directory of images"

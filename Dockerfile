# syntax=docker/dockerfile:1

ARG GO_VERSION=1.21
ARG ALPINE_VERSION=3.18

FROM golang:${GO_VERSION} AS build

WORKDIR /usr/src

COPY go.* .
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w -extldflags '-static'" \
    -buildvcs=false \
    -tags osusergo,netgo \
    -o /usr/bin/ ./...

FROM alpine:${ALPINE_VERSION} AS runtime

WORKDIR /opt

ARG LITEFS_CONFIG="litefs.yml"
ENV LITEFS_DIR="/litefs"
ENV DATABASE_URL="${LITEFS_DIR}/yap.db"
ENV INTERNAL_PORT=8080
ENV PORT=8081

# copy binary from build
COPY --from=build /usr/bin/yap ./a
COPY --from=build /usr/bin/sqlite3 ./b

# install sqlite, ca-certificates, curl and fuse for litefs
RUN apk add --no-cache fuse3 sqlite ca-certificates

# prepar for litefs
COPY --from=flyio/litefs:0.5 /usr/local/bin/litefs /usr/local/bin/litefs
ADD litefs/${LITEFS_CONFIG} /etc/litefs.yml

ENTRYPOINT ["litefs", "mount"]
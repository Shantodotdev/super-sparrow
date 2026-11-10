# Builds the server image: the Go binary and the built frontend it serves. Build it from the
# repository root: docker build -f deploy/Dockerfile --build-arg VERSION=v0.2.0 .

FROM oven/bun:1.4.4 AS web
WORKDIR /src
COPY package.json bun.lock ./
COPY web/package.json web/
COPY desktop/package.json desktop/
RUN bun install --frozen-lockfile
COPY tsconfig.base.json .gitignore ./
COPY web web
RUN bun --filter @super-sparrow/web build
COPY deploy/verify-web-build.ts deploy/
RUN bun deploy/verify-web-build.ts

FROM golang:1.27.3-bookworm AS server
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server ./
ARG VERSION
RUN test -n "$VERSION" || { echo "pass --build-arg VERSION=..."; exit 1; }
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/sparrow ./cmd/sparrow
# The image has no shell, so make the data folder here for the nonroot user to own.
RUN mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /out/sparrow /usr/local/bin/sparrow
COPY --from=web /src/web/dist/client /srv/web
COPY --from=server --chown=nonroot:nonroot /out/data /data
VOLUME /data
EXPOSE 8090
ENTRYPOINT ["/usr/local/bin/sparrow"]
CMD ["serve"]

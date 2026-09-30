# Controller (dashboard) image: builds the web UI, embeds it, ships one binary.
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
# web/public/install.sh is a symlink to ../../install.sh
COPY install.sh /src/install.sh
RUN npm run build

FROM golang:1.27-alpine AS server
WORKDIR /src
COPY agent/ ./agent/
COPY server/ ./server/
COPY --from=web /src/web/dist/ ./server/internal/webui/dist/
ARG VERSION=dev
RUN cd server && CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/cockpit-server ./cmd/cockpit-server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=server /out/cockpit-server /usr/local/bin/cockpit-server
ENV COCKPIT_DB=/data/cockpit.db \
    COCKPIT_LISTEN=0.0.0.0:8080
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["cockpit-server"]

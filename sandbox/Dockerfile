FROM node:22-bookworm AS node

FROM golang:1.25-bookworm

COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        git \
        g++ \
        make \
        procps \
        python3 \
        python3-pip \
        python3-venv \
        ripgrep \
        tini \
    && rm -rf /var/lib/apt/lists/* \
    && ln -s /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -s /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
    && ln -s /usr/local/lib/node_modules/corepack/dist/corepack.js /usr/local/bin/corepack \
    && useradd --create-home --uid 1000 --shell /bin/sh perpetual

ENV HOME=/home/perpetual \
    GOPATH=/home/perpetual/go \
    PATH=/home/perpetual/go/bin:/workspace/node_modules/.bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin

USER perpetual
WORKDIR /workspace
ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["sleep", "infinity"]

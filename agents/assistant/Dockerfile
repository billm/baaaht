# syntax=docker/dockerfile:1

ARG BASE_IMAGE=ghcr.io/billm/baaaht/agent-base:sha-local

FROM ${BASE_IMAGE}

WORKDIR /app

USER root

COPY --chown=1000:1000 package.json package-lock.json* ./
RUN npm ci --include=dev --ignore-scripts

COPY --chown=1000:1000 tsconfig.json ./
COPY --chown=1000:1000 src ./src

ENV TMPDIR=/tmp
ENV TSX_DISABLE_CACHE=1

RUN chown -R 1000:1000 /app

USER 1000:1000

EXPOSE 3000

CMD ["node_modules/.bin/tsx", "src/index.ts"]

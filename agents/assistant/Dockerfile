# syntax=docker/dockerfile:1

ARG BASE_IMAGE=ghcr.io/billm/baaaht/agent-base:sha-local

FROM ${BASE_IMAGE} AS builder

WORKDIR /app

USER root

COPY --chown=1000:1000 package.json package-lock.json* ./
RUN npm ci --include=dev --ignore-scripts

COPY --chown=1000:1000 tsconfig.json ./
COPY --chown=1000:1000 src ./src

RUN npm run build
RUN npm prune --omit=dev

FROM ${BASE_IMAGE}

WORKDIR /app

USER root

COPY --from=builder --chown=1000:1000 /app/package.json ./
COPY --from=builder --chown=1000:1000 /app/node_modules ./node_modules
COPY --from=builder --chown=1000:1000 /app/dist ./dist

ENV TMPDIR=/tmp

RUN chown -R 1000:1000 /app

USER 1000:1000

EXPOSE 3000

CMD ["node", "dist/index.js"]

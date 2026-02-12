# Multi-stage build for the LLM Gateway container.
# Produces a minimal image that runs as non-root (UID 1000) on read-only rootfs.

# ---- build stage ----
FROM node:22-alpine AS builder

WORKDIR /build

COPY package.json package-lock.json* ./
RUN npm ci --ignore-scripts

COPY tsconfig.json ./
COPY src/ src/

RUN npx tsc

# Prune dev dependencies for the production image
RUN npm prune --production

# ---- runtime stage ----
FROM node:22-alpine

# wget is required by the Docker HEALTHCHECK (used by the orchestrator).
# node:22-alpine already has UID/GID 1000 (node:node), so we reuse it.
RUN apk add --no-cache wget

WORKDIR /app

# Copy only production node_modules and compiled JS
COPY --from=builder --chown=1000:1000 /build/node_modules ./node_modules
COPY --from=builder --chown=1000:1000 /build/dist ./dist
COPY --from=builder --chown=1000:1000 /build/package.json ./package.json

# Health check matching the orchestrator's container config
HEALTHCHECK --interval=5s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -O- -q http://localhost:8080/health || exit 1

USER 1000:1000

EXPOSE 8080

CMD ["node", "dist/index.js"]

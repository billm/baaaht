# syntax=docker/dockerfile:1

FROM node:22-alpine

RUN apk add --no-cache tini

WORKDIR /app

ENV NODE_ENV=production

USER 1000:1000

ENTRYPOINT ["/sbin/tini", "--"]

# syntax=docker/dockerfile:1.7

FROM node:22-alpine AS dependencies
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci

FROM node:22-alpine AS build
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1
# Inlined into the client bundle by `next build`, so it has to be a build arg:
# a runtime environment variable never reaches the browser.
ARG NEXT_PUBLIC_NEURUN_API_URL=http://localhost:1267
ENV NEXT_PUBLIC_NEURUN_API_URL=$NEXT_PUBLIC_NEURUN_API_URL
# The GitHub App slug its install URL is keyed by. Inlined at build time like
# every NEXT_PUBLIC_ value, so it has to be present here rather than at runtime.
ARG NEXT_PUBLIC_NEURUN_GITHUB_APP_SLUG=
ENV NEXT_PUBLIC_NEURUN_GITHUB_APP_SLUG=$NEXT_PUBLIC_NEURUN_GITHUB_APP_SLUG
COPY --from=dependencies /app/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:22-alpine AS runtime
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV HOSTNAME=0.0.0.0
ENV PORT=3001

RUN addgroup --system --gid 1001 nodejs \
    && adduser --system --uid 1001 --ingroup nodejs nextjs

COPY --from=build --chown=nextjs:nodejs /app/public ./public
COPY --from=build --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=build --chown=nextjs:nodejs /app/.next/static ./.next/static

USER nextjs
EXPOSE 3001
CMD ["node", "server.js"]

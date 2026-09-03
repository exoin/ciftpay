# CiftPay web (Next.js 15 PWA). Build context: ./web, but the OpenAPI contract
# lives one level up, so compose passes `context: .` with this dockerfile —
# see docker-compose.yml.
FROM node:22-alpine AS deps
WORKDIR /app
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund

FROM node:22-alpine AS build
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1
ARG NEXT_PUBLIC_API_BASE_URL=http://localhost:8080
ARG NEXT_PUBLIC_WEB_BASE_URL=http://localhost:3000
ENV NEXT_PUBLIC_API_BASE_URL=$NEXT_PUBLIC_API_BASE_URL
ENV NEXT_PUBLIC_WEB_BASE_URL=$NEXT_PUBLIC_WEB_BASE_URL
COPY --from=deps /app/node_modules ./node_modules
COPY api/openapi.yaml /api/openapi.yaml
COPY web/ ./
RUN npm run gen:api && npm run build

FROM node:22-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 PORT=3000 HOSTNAME=0.0.0.0
RUN addgroup -S app && adduser -S -G app app
COPY --from=build --chown=app:app /app/.next/standalone ./
COPY --from=build --chown=app:app /app/.next/static ./.next/static
COPY --from=build --chown=app:app /app/public ./public
USER app
EXPOSE 3000
HEALTHCHECK --interval=15s --timeout=3s --retries=5 CMD wget -qO- http://127.0.0.1:3000/manifest.webmanifest >/dev/null || exit 1
CMD ["node", "server.js"]

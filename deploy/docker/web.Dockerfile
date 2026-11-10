# Builds the SvelteKit SPA and serves it with nginx (unprivileged, port 8080).
FROM node:24-alpine AS build
RUN corepack enable
WORKDIR /src
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json apps/web/
RUN pnpm install --frozen-lockfile --filter @laamo/web
COPY apps/web/ apps/web/
RUN pnpm --filter @laamo/web build

FROM nginxinc/nginx-unprivileged:1.27-alpine
COPY deploy/docker/web.nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/apps/web/build /usr/share/nginx/html
EXPOSE 8080

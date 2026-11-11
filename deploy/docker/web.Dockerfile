# Builds the SvelteKit SPA and serves it with nginx (unprivileged, port 8080).
FROM node:24-alpine AS build
RUN corepack enable
WORKDIR /src
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json apps/web/
COPY packages/catalog/package.json packages/catalog/
RUN pnpm install --frozen-lockfile --filter @laamo/web...
COPY packages/catalog/ packages/catalog/
COPY apps/web/ apps/web/
# The project URL and its PUBLIC key go into the bundle (sign-in talks to
# Eurobase auth directly). Never pass the secret key here.
ARG EUROBASE_URL=https://laamo.eurobase.app
ARG EUROBASE_PUBLIC_KEY=
ENV EUROBASE_URL=$EUROBASE_URL EUROBASE_PUBLIC_KEY=$EUROBASE_PUBLIC_KEY
RUN pnpm --filter @laamo/web build

FROM nginxinc/nginx-unprivileged:1.27-alpine
COPY deploy/docker/web.nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/apps/web/build /usr/share/nginx/html
EXPOSE 8080

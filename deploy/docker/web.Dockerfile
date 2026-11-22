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
# The public site's URL, and SITE_INDEX=1 only for production (search engines).
ARG SITE_URL=https://laamo.app
ARG SITE_INDEX=
ENV EUROBASE_URL=$EUROBASE_URL EUROBASE_PUBLIC_KEY=$EUROBASE_PUBLIC_KEY SITE_URL=$SITE_URL SITE_INDEX=$SITE_INDEX
RUN pnpm --filter @laamo/web build

FROM nginxinc/nginx-unprivileged:1.27-alpine
# Where /api goes (set per environment; the default suits docker run locally).
ENV API_UPSTREAM=http://host.docker.internal:8787
COPY deploy/docker/web.nginx.conf.template /etc/nginx/templates/default.conf.template
# Optional login in front of the site (BASIC_AUTH_HTPASSWD, set on staging).
COPY --chmod=755 deploy/docker/web-basic-auth.envsh /docker-entrypoint.d/15-basic-auth.envsh
COPY --from=build /src/apps/web/build /usr/share/nginx/html
EXPOSE 8080

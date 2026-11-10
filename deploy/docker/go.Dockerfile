# Builds one Go service. Usage: --build-arg CMD=api|worker|mailin
# Build context: the repo root (the backend embeds packages/catalog and packages/openapi).
FROM golang:1.26-alpine AS build
ARG CMD
WORKDIR /src/backend
COPY packages/catalog/go.mod packages/catalog/*.go packages/catalog/bricks.json /src/packages/catalog/
COPY packages/catalog/templates/ /src/packages/catalog/templates/
COPY packages/openapi/go.mod packages/openapi/*.go packages/openapi/openapi.yaml /src/packages/openapi/
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN test -n "$CMD" && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${CMD}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]

# Builds one Go service. Usage: --build-arg CMD=api|worker|mailin
FROM golang:1.26-alpine AS build
ARG CMD
WORKDIR /src
COPY backend/go.mod ./
RUN go mod download
COPY backend/ ./
RUN test -n "$CMD" && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${CMD}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]

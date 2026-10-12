FROM node:22-bookworm AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/frontend/dist ./frontend/dist
RUN go build -o /out/web ./cmd/web

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
ENV SPA_ASSET_DIR=/app/static
COPY --from=build /out/web /usr/local/bin/jobs-web
COPY --from=build /src/templates /app/templates
COPY --from=build /src/frontend/dist /app/static
EXPOSE 8080
CMD ["jobs-web"]

FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o /out/admin ./cmd/admin

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl sqlite3 && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/admin /usr/local/bin/jobs-admin
EXPOSE 8081
CMD ["jobs-admin"]

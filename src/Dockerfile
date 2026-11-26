# Build the SPA before embedding it into the Go binary
FROM node:24-bookworm-slim AS frontend-build

WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --include=optional --no-audit --no-fund
COPY frontend ./
RUN npm run build

# compile
FROM golang:1.27.1-alpine AS build

RUN apk add --no-cache gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
COPY src ./src
COPY web ./web
COPY --from=frontend-build /app/web/dist ./web/dist

RUN CGO_ENABLED=1 go build -o /server ./src/cmd/server
RUN CGO_ENABLED=1 go build -o /worker ./src/cmd/worker

# run
FROM alpine:3.20

WORKDIR /app

COPY --from=build /server ./server
COPY --from=build /worker ./worker

ENV APP_ADDR=:8080
EXPOSE 8080

CMD ["./server"]

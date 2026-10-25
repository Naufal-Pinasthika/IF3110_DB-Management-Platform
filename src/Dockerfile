# compile
FROM golang:1.27.1-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
COPY src ./src

RUN CGO_ENABLED=0 go build -o /server ./src/cmd/server

# run
FROM alpine:3.20

WORKDIR /app

COPY --from=build /server ./server

ENV APP_ADDR=:8080
EXPOSE 8080

CMD ["./server"]

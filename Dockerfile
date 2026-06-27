FROM golang:1.26-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /plugin .

FROM gcr.io/distroless/static-debian12

COPY --from=builder /plugin /plugin

EXPOSE 50051

ENTRYPOINT ["/plugin"]

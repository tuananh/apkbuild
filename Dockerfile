# Build the frontend binary
FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /frontend ./cmd/frontend

# Frontend image: run as BuildKit gateway
FROM alpine:3.23
RUN apk add --no-cache ca-certificates
COPY --from=builder /frontend /frontend
ENTRYPOINT ["/frontend"]

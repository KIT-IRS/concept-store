# Stage 1: Build
FROM golang:1.25.3 AS builder

WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -v -o concept-store

# Stage 2: Run
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/concept-store .

EXPOSE 3737
CMD ["./concept-store"]
# ── Stage 1: Build ──────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o scheduler .

# ── Stage 2: Run ────────────────────────────────────────────────────────────
FROM alpine:3.19

WORKDIR /app

RUN addgroup -S app && adduser -S app -G app

COPY --from=builder /app/scheduler .
COPY config.yaml .

RUN mkdir -p data && chown -R app:app /app

USER app

EXPOSE 8080

CMD ["./scheduler"]

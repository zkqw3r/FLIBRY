FROM golang:alpine AS builder

WORKDIR /app

# Copy dependency files
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Copy backend source code
COPY backend/ .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/main ./cmd/app/main.go

# Final image
FROM alpine:latest

WORKDIR /app

# Add CA certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Copy the binary from the builder stage
COPY --from=builder /app/main .

# Copy frontend templates and static files
COPY frontentd /frontentd

# Create directory for book storage
RUN mkdir -p /app/storage/books

EXPOSE 8080

CMD ["./main"]
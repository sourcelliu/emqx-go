# Stage 1: Build the application
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum to download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the application
# CGO_ENABLED=0 is important for a static binary, making it portable.
# -o /app/emqx-go specifies the output file name.
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/emqx-go ./cmd/emqx-go

# Stage 2: Create the final, minimal image
FROM alpine:latest

WORKDIR /root/

# Copy the built binary from the builder stage
COPY --from=builder /app/emqx-go .

# Expose the MQTT port
EXPOSE 1883

# Command to run the application
CMD ["./emqx-go"]
# --- Build Stage ---
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files to download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the application
# CGO_ENABLED=0 is important for a static binary, which is good for scratch images.
# -o /app/emqx-go creates the binary at a known location.
RUN CGO_ENABLED=0 go build -o /app/emqx-go ./cmd/emqx-go

# --- Final Stage ---
FROM scratch

# Copy the binary from the builder stage
COPY --from=builder /app/emqx-go /emqx-go

# Expose the default MQTT port
EXPOSE 1883

# Set the entrypoint for the container
ENTRYPOINT ["/emqx-go"]
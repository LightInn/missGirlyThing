# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o missgirlything .

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /build/missgirlything .

# Create data directory for persistent storage
RUN mkdir -p /app/data

# Set environment variables with defaults
ENV DISCORD_TOKEN="" \
    WORD_LIST="shit,fuck,damn,ass,bitch,idiot,stupid" \
    GIF_DISPLAY_SECONDS=3

# Volume for persistent data
VOLUME ["/app/data"]

# Run the bot
CMD ["./missgirlything"]

FROM golang:1.26-bookworm AS build
WORKDIR /src

# Install system dependencies for CGO and ONNX
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# Copy pre-built onnxruntime libraries from build context
COPY vendor/libs/libonnxruntime.so* /usr/local/lib/
COPY vendor/libs/libtokenizers.a /usr/local/lib/

# Create onnxruntime include directory (minimal headers for linking)
RUN mkdir -p /usr/local/include/onnxruntime

# Update library cache
RUN ldconfig

# Copy Go module files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build with CGO enabled
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -tags cgo -mod=mod -o /out/iris-bot ./cmd/iris-bot

FROM debian:bookworm-slim AS runtime
WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    curl \
    gnupg \
    python3 \
    python3-pip \
    python3-venv \
    pipx \
    && curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && npm install -g npm@latest \
    && pip3 install --no-cache-dir --break-system-packages uv \
    && apt-get purge -y --auto-remove curl gnupg \
    && rm -rf /var/lib/apt/lists/*

# Copy onnxruntime libraries from build stage
COPY --from=build /usr/local/lib/libonnxruntime.so* /usr/local/lib/

# Copy binary from build stage
COPY --from=build /out/iris-bot /usr/local/bin/iris-bot

# Update library cache
RUN ldconfig

# Create non-root user with UID 1000 to match common host UIDs (so a
# bind-mounted ./config from the host is writable from inside the
# container without an extra chown).
RUN groupadd --system --gid 1000 app && useradd --system --uid 1000 --gid 1000 --create-home app

RUN mkdir -p /app/config /opt/iris-models /home/app/.npm /home/app/.cache/uv \
    && chown -R app:app /app /opt/iris-models /home/app

USER app
ENV HOME=/home/app \
    NPM_CONFIG_CACHE=/home/app/.npm \
    UV_CACHE_DIR=/home/app/.cache/uv \
    PATH=/home/app/.local/bin:${PATH}
ENTRYPOINT ["/usr/local/bin/iris-bot"]

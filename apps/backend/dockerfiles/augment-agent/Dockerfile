# Augment Agent Dockerfile
# A coding agent powered by Auggie CLI (Augment Code) with agentctl sidecar

# Stage 1: Build agentctl from Go source
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build agentctl binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /agentctl ./cmd/agentctl

# Stage 2: Final image with Node.js and agentctl
FROM node:20-slim

# Install dependencies
RUN apt-get update && apt-get install -y \
    git \
    curl \
    jq \
    python3 \
    python3-pip \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install Auggie CLI from npm
RUN npm install -g @augmentcode/auggie

# Copy agentctl binary from builder
COPY --from=builder /agentctl /usr/local/bin/agentctl
RUN chmod +x /usr/local/bin/agentctl

# Create agent directory
WORKDIR /agent

# Copy agent script (legacy, for backward compatibility)
COPY dockerfiles/augment-agent/agent.sh /agent/agent.sh
RUN chmod +x /agent/agent.sh

# Copy entrypoint script
COPY dockerfiles/augment-agent/entrypoint.sh /agent/entrypoint.sh
RUN chmod +x /agent/entrypoint.sh

# Workspace will be mounted here
VOLUME /workspace
WORKDIR /workspace

# Environment variables (passed at runtime)
# TASK_ID - The task identifier
# TASK_TITLE - Task title/name
# TASK_DESCRIPTION - Full task description
# AUGMENT_SESSION_AUTH - Session auth JSON for Augment (from ~/.augment/session.json)
# AUGGIE_SESSION_ID - (Optional) Session ID to resume a previous conversation
#
# agentctl configuration:
# AGENTCTL_PORT - HTTP API port (default: 9999)
# AGENTCTL_AGENT_COMMAND - Command to run (default: "auggie --acp")
# AGENTCTL_AUTO_START - Auto-start agent on startup (default: false)
# AGENTCTL_MODE - "agentctl" for HTTP mode, "direct" for legacy stdin/stdout mode

# Expose agentctl HTTP API port
EXPOSE 9999

# Default entrypoint that decides between agentctl and direct mode
ENTRYPOINT ["/agent/entrypoint.sh"]


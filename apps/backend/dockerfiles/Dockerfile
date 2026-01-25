# Kandev Multi-Agent Dockerfile
# A multi-agent image supporting Auggie CLI (Augment Code) and Codex CLI (OpenAI) with agentctl sidecar
#
# Prerequisites: Build agentctl before building this image:
#   make build-agentctl
#
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

# Install Codex CLI from npm
RUN npm install -g @openai/codex

# Copy pre-built agentctl binary (run `make build-agentctl` first)
COPY bin/agentctl /usr/local/bin/agentctl
RUN chmod +x /usr/local/bin/agentctl

# Create agent directory
WORKDIR /agent

# Copy entrypoint script
COPY dockerfiles/entrypoint.sh /agent/entrypoint.sh
RUN chmod +x /agent/entrypoint.sh

# Workspace will be mounted here
VOLUME /workspace
WORKDIR /workspace

# Environment variables (passed at runtime)
# TASK_ID - The task identifier
# TASK_TITLE - Task title/name
# TASK_DESCRIPTION - Full task description
#
# For Auggie:
# AUGMENT_SESSION_AUTH - Session auth JSON for Augment (from ~/.augment/session.json)
# AUGGIE_SESSION_ID - (Optional) Session ID to resume a previous conversation
#
# For Codex:
# OPENAI_API_KEY - OpenAI API key for Codex
#
# agentctl configuration:
# AGENTCTL_PORT - HTTP API port (default: 9999)
# AGENTCTL_AGENT_COMMAND - Command to run (e.g., "auggie --acp" or "codex app-server")
# AGENTCTL_AUTO_START - Auto-start agent on startup (default: false)

# Expose agentctl HTTP API port
EXPOSE 9999

# Run agentctl HTTP server
ENTRYPOINT ["/agent/entrypoint.sh"]


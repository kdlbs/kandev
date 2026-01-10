# Augment Agent Dockerfile
# A coding agent powered by Auggie CLI (Augment Code)

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

# Create agent directory
WORKDIR /agent

# Copy agent script
COPY agent.sh /agent/agent.sh
RUN chmod +x /agent/agent.sh

# Workspace will be mounted here
VOLUME /workspace
WORKDIR /workspace

# Environment variables (passed at runtime)
# TASK_ID - The task identifier
# TASK_TITLE - Task title/name
# TASK_DESCRIPTION - Full task description
# AUGMENT_SESSION_AUTH - Session auth JSON for Augment (from ~/.augment/session.json)
# AUGGIE_SESSION_ID - (Optional) Session ID to resume a previous conversation

# Output ACP messages to stdout
ENTRYPOINT ["/agent/agent.sh"]


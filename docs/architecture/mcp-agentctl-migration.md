# MCP Server Migration: Backend → agentctl

This document describes the architectural change to move the MCP server from the Kandev backend to agentctl, enabling remote agents to access MCP tools.

## Problem Statement

Currently, the MCP server runs embedded in the Kandev backend on the user's machine. When agents run remotely (Docker containers, VPS, Kubernetes), they cannot access the MCP server because it's not reachable from their network location.

## Solution

Move the MCP server into agentctl, which always runs co-located with the agent. API calls from MCP tools are tunneled back to the Kandev backend through the existing WebSocket connection.

---

## Current Architecture (MCP Server with Backend)

```mermaid
flowchart TB
    subgraph UserMachine["👤 USER'S MACHINE"]
        subgraph Backend["KANDEV BACKEND"]
            Orchestrator["Orchestrator"]
            Lifecycle["Lifecycle Manager"]
            Runtime["Runtime<br/>(Docker/Standalone)"]

            subgraph MCPServer["EMBEDDED MCP SERVER :9090"]
                SSE["/sse endpoint"]
                MCP["/mcp endpoint"]
                Tools["Tools:<br/>list_workspaces<br/>list_tasks<br/>create_task<br/>update_task"]
            end

            API["REST API :8080<br/>/api/v1/tasks<br/>/api/v1/boards"]

            Orchestrator --> Lifecycle
            Lifecycle --> Runtime
            MCPServer -->|"HTTP calls"| API
        end
    end

    subgraph Remote["🖥️ REMOTE SERVER / CONTAINER"]
        subgraph Agentctl["AGENTCTL"]
            InstanceMgr["Instance Manager<br/>Ports: 10000+"]
            Agent["Agent Process<br/>(Claude, Codex, etc)<br/>stdin/stdout ACP"]

            InstanceMgr --> Agent
        end

        NoMCP["🚫 NO MCP ACCESS<br/>Port 9090 unreachable"]
        Agent -.->|"❌ CANNOT CONNECT"| NoMCP
    end

    Runtime -->|"WS connection"| Agentctl
    NoMCP -.->|"❌ Network barrier"| MCPServer

    style NoMCP fill:#ffcccc,stroke:#cc0000
    style MCPServer fill:#ffffcc,stroke:#cccc00
```

---

## Proposed Architecture (MCP Server in agentctl)

```mermaid
flowchart TB
    subgraph UserMachine["👤 USER'S MACHINE"]
        subgraph Backend["KANDEV BACKEND"]
            Orchestrator["Orchestrator"]
            Lifecycle["Lifecycle Manager"]
            Runtime["Runtime<br/>(Docker/Standalone)"]
            API["REST API :8080<br/>/api/v1/tasks<br/>/api/v1/boards"]
            WSServer["WebSocket Server"]

            Orchestrator --> Lifecycle
            Lifecycle --> Runtime
        end
    end

    subgraph Remote["🖥️ REMOTE SERVER / CONTAINER"]
        subgraph Agentctl["AGENTCTL"]
            InstanceMgr["Instance Manager<br/>Ports: 10000+"]

            subgraph MCPServer["EMBEDDED MCP SERVER"]
                SSE["/sse endpoint"]
                MCP["/mcp endpoint"]
                Tools["Tools:<br/>list_workspaces<br/>list_tasks<br/>create_task<br/>update_task"]
            end

            WSTunnel["WS Tunnel Client<br/>(API Proxy)"]

            Agent["Agent Process<br/>(Claude, Codex, etc)<br/>stdin/stdout ACP"]

            InstanceMgr --> Agent
            MCPServer -->|"API calls"| WSTunnel
            Agent -->|"✅ localhost"| MCPServer
        end
    end

    Runtime <-->|"WS connection"| Agentctl
    WSTunnel <-->|"Tunneled API requests"| WSServer
    WSServer --> API

    style MCPServer fill:#ccffcc,stroke:#00cc00
    style WSTunnel fill:#cce5ff,stroke:#0066cc
```

---

## Sequence Diagram: Agent Using MCP (Proposed)

```mermaid
sequenceDiagram
    participant Agent as Agent Process
    participant MCP as MCP Server<br/>(in agentctl)
    participant Tunnel as WS Tunnel<br/>(in agentctl)
    participant WSHandler as WS Handler<br/>(in Backend)
    participant Service as Service Layer<br/>(in Backend)
    participant DB as Database

    Note over Agent,DB: Agent wants to create a task

    Agent->>MCP: POST /mcp<br/>tool: create_task
    MCP->>MCP: Parse MCP request

    MCP->>Tunnel: Forward request
    Tunnel->>WSHandler: WS message<br/>{type: "mcp_request", tool: "create_task", ...}
    WSHandler->>Service: taskService.Create(...)
    Service->>DB: Insert task
    DB-->>Service: Task created
    Service-->>WSHandler: Task entity
    WSHandler-->>Tunnel: WS message<br/>{type: "mcp_response", result: {...}}
    Tunnel-->>MCP: Response

    MCP-->>Agent: MCP tool result<br/>{task_id: "..."}

    Note over Agent,DB: ✅ Direct service calls, no HTTP overhead
```

---

## Component Comparison

```mermaid
flowchart LR
    subgraph Current["❌ CURRENT"]
        direction TB
        B1["Backend"]
        M1["MCP Server"]
        A1["agentctl"]
        AG1["Agent"]

        B1 --- M1
        B1 -.->|"WS"| A1
        A1 --- AG1
        AG1 -.->|"❌ unreachable"| M1
    end

    subgraph Proposed["✅ PROPOSED"]
        direction TB
        B2["Backend"]
        A2["agentctl"]
        M2["MCP Server"]
        AG2["Agent"]

        B2 <-->|"WS + Tunnel"| A2
        A2 --- M2
        A2 --- AG2
        AG2 -->|"✅ localhost"| M2
    end

    Current -->|"Migration"| Proposed

    style Current fill:#ffeeee
    style Proposed fill:#eeffee
```

---

## Summary

| Aspect | Current | Proposed |
|--------|---------|----------|
| **MCP Location** | Backend (user's machine) | agentctl (co-located with agent) |
| **Remote Agent Access** | ❌ Cannot reach MCP | ✅ Always localhost |
| **Network Dependency** | Requires direct network path | Works over existing WS tunnel |
| **Deployment Flexibility** | Limited to local agents | Works anywhere (Docker, VPS, K8s) |
| **API Communication** | MCP → localhost API | MCP → WS Tunnel → Backend API |

---

## Implementation Notes

### Changes Required

1. **agentctl**: Add embedded MCP server (reuse `internal/mcpserver` package)
2. **agentctl**: Add WS tunnel client for forwarding MCP tool calls to backend
3. **Backend**: Add WS handler for `mcp_request` messages that calls services directly
4. **Backend**: Remove embedded MCP server from `cmd/kandev`
5. **Lifecycle Manager**: Pass backend WS URL to agentctl during instance creation

### WS Tunnel Protocol

The tunnel uses the existing WebSocket connection with MCP-specific message types. The backend WS handler calls services directly (no HTTP overhead):

```json
{
  "type": "mcp_request",
  "id": "req-123",
  "tool": "create_task",
  "arguments": {
    "workspace_id": "ws-1",
    "board_id": "board-1",
    "title": "New task"
  }
}
```

Response:

```json
{
  "type": "mcp_response",
  "id": "req-123",
  "result": {
    "task_id": "task-456",
    "title": "New task"
  }
}
```

Error response:

```json
{
  "type": "mcp_response",
  "id": "req-123",
  "error": {
    "code": "NOT_FOUND",
    "message": "Board not found"
  }
}
```

### MCP Configuration for Agents

When configuring agents that support MCP, agentctl will provide the local MCP endpoint:

```json
{
  "mcpServers": {
    "kandev": {
      "url": "http://localhost:10001/mcp"
    }
  }
}
```

Where `10001` is the instance's allocated port.


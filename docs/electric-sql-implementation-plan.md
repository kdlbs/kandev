# Electric SQL Implementation Plan for Kandev

## Executive Summary

This document outlines a comprehensive plan to migrate kandev from WebSocket-based real-time synchronization to Electric SQL, based on the implementation patterns from vibe-kanban. This migration will provide:

- **Simplified Architecture**: Remove custom WebSocket hub, dispatcher, and event bus infrastructure
- **Offline-First**: Built-in local-first capabilities with automatic conflict resolution
- **Better Type Safety**: Database schema drives frontend types automatically
- **Reduced Latency**: Direct client-side SQLite queries without round-trip requests
- **Scalability**: PostgreSQL logical replication handles multiple clients efficiently

---

## Current Architecture vs. Electric SQL

### Current WebSocket Architecture

```
┌──────────────┐         WebSocket          ┌─────────────────┐
│   Frontend   │◄──────────────────────────►│    Backend      │
│  (React +    │   Custom Message Protocol  │   (Go + Gin)    │
│   Zustand)   │   Subscribe/Notify Pattern │                 │
└──────────────┘                            └────────┬────────┘
                                                     │
                                              ┌──────▼──────┐
                                              │   SQLite    │
                                              │  Database   │
                                              └─────────────┘
```

**Challenges**:
- Custom message routing and subscription management
- Manual state synchronization between frontend and backend
- No offline support or conflict resolution
- Backend must broadcast changes to all relevant subscribers
- Additional complexity with NATS event bus

### Electric SQL Architecture

```
┌──────────────┐                              ┌─────────────────┐
│   Frontend   │         HTTP Proxy           │    Backend      │
│  (React +    │◄────────────────────────────►│   (Go + Gin)    │
│   Electric)  │   Shape Subscriptions        │                 │
└──────┬───────┘                              └────────┬────────┘
       │                                               │
   ┌───▼────────┐                              ┌──────▼──────┐
   │  wa-sqlite │                              │ PostgreSQL  │
   │   (WASM)   │                              │  + Electric │
   └────────────┘                              └──────┬──────┘
                                                      │
                                               ┌──────▼──────┐
                                               │  Electric   │
                                               │   Service   │
                                               └─────────────┘
```

**Benefits**:
- Electric handles all synchronization automatically
- Client queries local SQLite database (instant reads)
- PostgreSQL logical replication provides real-time updates
- Built-in conflict resolution and offline support
- Security enforced via backend proxy with server-side WHERE clauses

---

## Implementation Phases

### Phase 1: Infrastructure Setup

#### 1.1 Database Migration (SQLite → PostgreSQL)

**Current**: SQLite (`/home/user/kandev/apps/backend/internal/task/repository/sqlite/`)

**Target**: PostgreSQL with Electric support

**Tasks**:

1. **Create PostgreSQL Schema**
   - Convert existing SQLite schema to PostgreSQL
   - File: Create `apps/backend/migrations/001_initial_schema.sql`
   - Enable logical replication on relevant tables

2. **Add Electric-Specific Configuration**
   - Create Electric sync role with replication privileges
   - Set up publication for logical replication
   - Create helper functions for table syncing

   ```sql
   -- Example from vibe-kanban pattern
   CREATE ROLE electric_sync WITH LOGIN REPLICATION;
   CREATE PUBLICATION electric_publication_default;

   CREATE OR REPLACE FUNCTION electric_sync_table(p_schema text, p_table text)
   RETURNS void AS $$
   BEGIN
       EXECUTE format('ALTER TABLE %I.%I REPLICA IDENTITY FULL', p_schema, p_table);
       EXECUTE format('GRANT SELECT ON TABLE %I.%I TO electric_sync', p_schema, p_table);
       EXECUTE format('ALTER PUBLICATION electric_publication_default ADD TABLE %I.%I',
           p_schema, p_table);
   END;
   $$ LANGUAGE plpgsql;
   ```

3. **Enable Electric Sync for Tables**
   ```sql
   SELECT electric_sync_table('public', 'workspaces');
   SELECT electric_sync_table('public', 'boards');
   SELECT electric_sync_table('public', 'tasks');
   SELECT electric_sync_table('public', 'task_sessions');
   SELECT electric_sync_table('public', 'task_session_messages');
   SELECT electric_sync_table('public', 'task_session_git_snapshots');
   SELECT electric_sync_table('public', 'task_session_commits');
   SELECT electric_sync_table('public', 'workflow_templates');
   SELECT electric_sync_table('public', 'workflow_steps');
   SELECT electric_sync_table('public', 'executors');
   SELECT electric_sync_table('public', 'environments');
   SELECT electric_sync_table('public', 'agent_profiles');
   SELECT electric_sync_table('public', 'users');
   ```

4. **Repository Layer Migration**
   - Replace SQLite repositories with PostgreSQL versions
   - Use `github.com/jackc/pgx/v5` (recommended) or `lib/pq`
   - Maintain same interface for minimal service layer changes

#### 1.2 Docker Compose Setup

**File**: Create `apps/backend/docker-compose.yml`

```yaml
version: '3.8'

services:
  postgres:
    image: postgres:16
    container_name: kandev-postgres
    environment:
      POSTGRES_DB: kandev
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-postgres}
      ELECTRIC_ROLE_PASSWORD: ${ELECTRIC_ROLE_PASSWORD:-electric123}
    ports:
      - "5432:5432"
    volumes:
      - postgres-data:/var/lib/postgresql/data
    command:
      - postgres
      - -c
      - wal_level=logical
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      timeout: 5s
      retries: 5

  electric:
    image: electricsql/electric:latest
    container_name: kandev-electric
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      DATABASE_URL: postgresql://electric_sync:${ELECTRIC_ROLE_PASSWORD:-electric123}@postgres:5432/kandev
      AUTH_MODE: insecure  # For development; change to secure for production
      ELECTRIC_INSECURE: "true"
      ELECTRIC_MANUAL_TABLE_PUBLISHING: "true"
      ELECTRIC_FEATURE_FLAGS: allow_subqueries,tagged_subqueries
      LOG_LEVEL: debug
    ports:
      - "3000:3000"
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://localhost:3000/v1/health || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  postgres-data:
```

**Configuration File**: `apps/backend/internal/config/config.go`

```go
type Config struct {
    // ... existing fields

    // Electric SQL Configuration
    ElectricURL          string `env:"ELECTRIC_URL" envDefault:"http://localhost:3000"`
    ElectricSecret       string `env:"ELECTRIC_SECRET"`
    ElectricRolePassword string `env:"ELECTRIC_ROLE_PASSWORD"`

    // PostgreSQL Configuration
    DatabaseURL          string `env:"DATABASE_URL" envDefault:"postgres://postgres:postgres@localhost:5432/kandev?sslmode=disable"`
}
```

#### 1.3 Frontend Dependencies

**File**: `apps/web/package.json`

Add dependencies:
```json
{
  "dependencies": {
    "@electric-sql/client": "^0.8.0",
    "@tanstack/react-db": "^0.1.50",
    "wa-sqlite": "^1.0.0"
  }
}
```

---

### Phase 2: Electric Proxy Implementation

Following vibe-kanban's security-first approach, implement a backend proxy that controls data access.

#### 2.1 Shape Definitions

**File**: Create `shared/electric/shapes.ts` (TypeScript types)

```typescript
export interface ShapeDefinition<T> {
    readonly table: string;
    readonly params: readonly string[];
    readonly url: string;
    readonly _type: T;  // Phantom field for type inference
}

function defineShape<T>(
    table: string,
    params: readonly string[],
    url: string
): ShapeDefinition<T> {
    return { table, params, url, _type: undefined as unknown as T };
}

// Workspace-scoped shapes
export const BOARDS_SHAPE = defineShape<Board>(
    'boards',
    ['workspace_id'] as const,
    '/v1/shape/workspace/{workspace_id}/boards'
);

export const TASKS_SHAPE = defineShape<Task>(
    'tasks',
    ['board_id'] as const,
    '/v1/shape/board/{board_id}/tasks'
);

// Session-scoped shapes
export const SESSION_MESSAGES_SHAPE = defineShape<SessionMessage>(
    'task_session_messages',
    ['session_id'] as const,
    '/v1/shape/session/{session_id}/messages'
);

export const SESSION_GIT_SNAPSHOTS_SHAPE = defineShape<GitSnapshot>(
    'task_session_git_snapshots',
    ['session_id'] as const,
    '/v1/shape/session/{session_id}/git-snapshots'
);

// User-scoped shapes
export const USER_WORKSPACES_SHAPE = defineShape<Workspace>(
    'workspaces',
    ['user_id'] as const,
    '/v1/shape/user/{user_id}/workspaces'
);
```

#### 2.2 Backend Proxy Routes

**File**: Create `apps/backend/internal/gateway/electric/proxy.go`

```go
package electric

import (
    "fmt"
    "net/http"
    "net/url"

    "github.com/gin-gonic/gin"
)

type ShapeDefinition struct {
    Table       string
    WhereClause string
    Params      []string
}

var shapes = map[string]ShapeDefinition{
    "/v1/shape/workspace/:workspace_id/boards": {
        Table:       "boards",
        WhereClause: `"workspace_id" = $1`,
        Params:      []string{"workspace_id"},
    },
    "/v1/shape/board/:board_id/tasks": {
        Table:       "tasks",
        WhereClause: `"board_id" = $1`,
        Params:      []string{"board_id"},
    },
    "/v1/shape/session/:session_id/messages": {
        Table:       "task_session_messages",
        WhereClause: `"task_session_id" = $1`,
        Params:      []string{"session_id"},
    },
    // ... more shapes
}

type Proxy struct {
    electricURL string
    httpClient  *http.Client
}

func NewProxy(electricURL string) *Proxy {
    return &Proxy{
        electricURL: electricURL,
        httpClient:  &http.Client{},
    }
}

func (p *Proxy) HandleShapeRequest(c *gin.Context) {
    // Get shape definition for this route
    routePattern := c.FullPath()
    shape, ok := shapes[routePattern]
    if !ok {
        c.JSON(http.StatusNotFound, gin.H{"error": "shape not found"})
        return
    }

    // TODO: Validate user has access to this resource
    // userID := c.GetString("user_id")
    // if !p.authorize(userID, shape, c.Params) { ... }

    // Build Electric URL with server-side table and WHERE clause
    electricURL, err := url.Parse(p.electricURL)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid electric URL"})
        return
    }
    electricURL.Path = "/v1/shape"

    // Add server-side parameters (security: client can't override)
    query := electricURL.Query()
    query.Set("table", shape.Table)
    query.Set("where", shape.WhereClause)

    // Add parameterized values from route params
    for i, paramName := range shape.Params {
        paramValue := c.Param(paramName)
        query.Set(fmt.Sprintf("params[%d]", i+1), paramValue)
    }

    // Forward safe client parameters (offset, handle, live, cursor, columns)
    safeParams := []string{"offset", "handle", "live", "cursor", "columns"}
    for _, param := range safeParams {
        if value := c.Query(param); value != "" {
            query.Set(param, value)
        }
    }

    electricURL.RawQuery = query.Encode()

    // Create request to Electric
    req, err := http.NewRequestWithContext(c.Request.Context(), "GET", electricURL.String(), nil)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
        return
    }

    // Forward auth headers if needed
    if auth := c.GetHeader("Authorization"); auth != "" {
        req.Header.Set("Authorization", auth)
    }

    // Execute request
    resp, err := p.httpClient.Do(req)
    if err != nil {
        c.JSON(http.StatusBadGateway, gin.H{"error": "electric request failed"})
        return
    }
    defer resp.Body.Close()

    // Stream response directly to client
    c.DataFromReader(resp.StatusCode, resp.ContentLength, resp.Header.Get("Content-Type"), resp.Body, nil)
}

// Register routes
func (p *Proxy) RegisterRoutes(router *gin.RouterGroup) {
    for route := range shapes {
        router.GET(route, p.HandleShapeRequest)
    }
}
```

**File**: `apps/backend/cmd/kandev/main.go`

```go
// Add to router setup
electricProxy := electric.NewProxy(cfg.ElectricURL)
api := r.Group("/api")
{
    electricProxy.RegisterRoutes(api)
    // ... existing routes
}
```

#### 2.3 Frontend Electric Client

**File**: Create `apps/web/lib/electric/client.ts`

```typescript
import { ShapeStreamOptions } from '@electric-sql/client';
import { ShapeDefinition } from '@shared/electric/shapes';

const BACKEND_API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080';

interface ShapeConfig extends Partial<ShapeStreamOptions> {
    onError?: (error: Error) => void;
}

export function createShapeStream<T>(
    shape: ShapeDefinition<T>,
    params: Record<string, string>,
    config?: ShapeConfig
) {
    // Build URL from shape definition and params
    let url = `${BACKEND_API_URL}${shape.url}`;
    for (const [key, value] of Object.entries(params)) {
        url = url.replace(`{${key}}`, encodeURIComponent(value));
    }

    return {
        url,
        headers: {
            Authorization: async () => {
                // TODO: Get auth token
                const token = localStorage.getItem('auth_token');
                return token ? `Bearer ${token}` : '';
            },
        },
        parser: {
            timestamptz: (value: string) => value,
        },
        fetchClient: fetch,
        onError: config?.onError,
        ...config,
    };
}
```

**File**: Create `apps/web/lib/electric/hooks.ts`

```typescript
import { useEffect, useState } from 'react';
import { Shape, ShapeStream } from '@electric-sql/client';
import { ShapeDefinition } from '@shared/electric/shapes';
import { createShapeStream } from './client';

export function useShape<T>(
    shape: ShapeDefinition<T>,
    params: Record<string, string>,
    enabled = true
) {
    const [data, setData] = useState<T[]>([]);
    const [isLoading, setIsLoading] = useState(true);
    const [error, setError] = useState<Error | null>(null);
    const [stream, setStream] = useState<ShapeStream<T> | null>(null);

    useEffect(() => {
        if (!enabled) {
            setIsLoading(false);
            return;
        }

        let cancelled = false;

        const config = createShapeStream(shape, params, {
            onError: (err) => {
                if (!cancelled) {
                    setError(err);
                    setIsLoading(false);
                }
            },
        });

        const shapeStream = new ShapeStream<T>(config);
        setStream(shapeStream);

        // Subscribe to changes
        const unsubscribe = shapeStream.subscribe((messages) => {
            if (cancelled) return;

            // Update local state based on messages
            messages.forEach((message) => {
                if (message.headers.operation === 'insert') {
                    setData((prev) => [...prev, message.value]);
                } else if (message.headers.operation === 'update') {
                    setData((prev) =>
                        prev.map((item) =>
                            (item as any).id === message.key
                                ? { ...item, ...message.value }
                                : item
                        )
                    );
                } else if (message.headers.operation === 'delete') {
                    setData((prev) =>
                        prev.filter((item) => (item as any).id !== message.key)
                    );
                }
            });

            setIsLoading(false);
        });

        return () => {
            cancelled = true;
            unsubscribe();
            shapeStream.unsubscribe();
        };
    }, [shape, JSON.stringify(params), enabled]);

    return { data, isLoading, error, stream };
}
```

---

### Phase 3: Mutation Handling

Electric SQL is read-only on the client. Mutations must go through the backend.

#### 3.1 Backend Mutation Endpoints

**File**: `apps/backend/internal/gateway/mutations/tasks.go`

```go
package mutations

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
)

type CreateTaskRequest struct {
    ID             *string `json:"id"`
    WorkspaceID    string  `json:"workspace_id" binding:"required"`
    BoardID        string  `json:"board_id" binding:"required"`
    WorkflowStepID string  `json:"workflow_step_id" binding:"required"`
    Title          string  `json:"title" binding:"required"`
    Description    string  `json:"description"`
}

type MutationResponse struct {
    Data interface{} `json:"data"`
    TxID int64       `json:"txid"`  // PostgreSQL transaction ID
}

func (h *TaskHandler) CreateTask(c *gin.Context) {
    var req CreateTaskRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // Generate ID if not provided (for optimistic updates)
    if req.ID == nil {
        id := uuid.New().String()
        req.ID = &id
    }

    // Start transaction
    tx, err := h.db.BeginTx(c.Request.Context(), nil)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start transaction"})
        return
    }
    defer tx.Rollback()

    // Create task
    task, err := h.taskRepo.Create(tx, &domain.Task{
        ID:             *req.ID,
        WorkspaceID:    req.WorkspaceID,
        BoardID:        req.BoardID,
        WorkflowStepID: req.WorkflowStepID,
        Title:          req.Title,
        Description:    req.Description,
    })
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
        return
    }

    // Get transaction ID for Electric sync tracking
    var txid int64
    err = tx.QueryRow("SELECT pg_current_xact_id()::text::bigint").Scan(&txid)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get txid"})
        return
    }

    if err := tx.Commit(); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to commit transaction"})
        return
    }

    // Return task with txid for optimistic update reconciliation
    c.JSON(http.StatusCreated, MutationResponse{
        Data: task,
        TxID: txid,
    })
}
```

#### 3.2 Frontend Mutation Hooks

**File**: Create `apps/web/lib/electric/mutations.ts`

```typescript
import { useMutation, useQueryClient } from '@tanstack/react-query';

interface MutationOptions {
    optimisticUpdate?: boolean;
}

export function useMutateTask(options: MutationOptions = {}) {
    const queryClient = useQueryClient();

    const createTask = useMutation({
        mutationFn: async (data: CreateTaskRequest) => {
            const response = await fetch(`${API_URL}/api/tasks`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${getAuthToken()}`,
                },
                body: JSON.stringify(data),
            });

            if (!response.ok) {
                throw new Error('Failed to create task');
            }

            return response.json();
        },
        onMutate: async (newTask) => {
            if (!options.optimisticUpdate) return;

            // Cancel outgoing refetches
            await queryClient.cancelQueries({ queryKey: ['tasks', newTask.board_id] });

            // Snapshot previous value
            const previousTasks = queryClient.getQueryData(['tasks', newTask.board_id]);

            // Optimistically update
            queryClient.setQueryData(['tasks', newTask.board_id], (old: any[]) => [
                ...old,
                { ...newTask, id: newTask.id || crypto.randomUUID() },
            ]);

            return { previousTasks };
        },
        onError: (err, newTask, context) => {
            // Rollback on error
            if (context?.previousTasks) {
                queryClient.setQueryData(
                    ['tasks', newTask.board_id],
                    context.previousTasks
                );
            }
        },
        onSuccess: (response) => {
            // Electric will sync the change automatically
            // The txid can be used to track when the change is confirmed
            console.log('Task created with txid:', response.txid);
        },
    });

    return { createTask };
}
```

---

### Phase 4: Replace WebSocket Handlers

#### 4.1 Migration Strategy

**Current WebSocket handlers** in `apps/web/lib/ws/handlers/*.ts` will be replaced by Electric shapes:

| Current WebSocket Event | Electric Shape | Notes |
|------------------------|----------------|-------|
| `task.created`, `task.updated`, `task.deleted` | `TASKS_SHAPE` | Real-time via Electric |
| `session.message.added/updated` | `SESSION_MESSAGES_SHAPE` | Real-time via Electric |
| `session.git.event` | `SESSION_GIT_SNAPSHOTS_SHAPE` + `SESSION_COMMITS_SHAPE` | Split into two shapes |
| `board.created/updated/deleted` | `BOARDS_SHAPE` | Real-time via Electric |
| `workspace.created/updated/deleted` | `USER_WORKSPACES_SHAPE` | Real-time via Electric |
| `session.state_changed` | `TASK_SESSIONS_SHAPE` | New shape needed |
| `session.shell.output`, `session.process.output` | Not synced via Electric | Keep WebSocket for streaming output |

**Hybrid Approach**:
- Use Electric for **database-backed data** (tasks, messages, git history, etc.)
- Keep WebSocket for **ephemeral streaming data** (shell output, process status, progress indicators)

#### 4.2 Context Refactoring

**File**: `apps/web/contexts/TaskContext.tsx`

Before (WebSocket):
```typescript
export function TaskProvider({ boardId, children }) {
    const { data: tasks } = useWebSocketTasks(boardId);
    // ...
}
```

After (Electric):
```typescript
import { useShape } from '@/lib/electric/hooks';
import { TASKS_SHAPE } from '@shared/electric/shapes';

export function TaskProvider({ boardId, children }) {
    const { data: tasks, isLoading } = useShape(
        TASKS_SHAPE,
        { board_id: boardId }
    );

    const { createTask, updateTask, deleteTask } = useMutateTask({
        optimisticUpdate: true,
    });

    return (
        <TaskContext.Provider value={{
            tasks,
            isLoading,
            createTask,
            updateTask,
            deleteTask,
        }}>
            {children}
        </TaskContext.Provider>
    );
}
```

#### 4.3 Remove WebSocket Infrastructure

Files to remove or deprecate:
- `apps/web/lib/ws/client.ts` - WebSocket client
- `apps/web/lib/ws/handlers/*.ts` - All handlers (except streaming)
- `apps/web/lib/ws/router.ts` - Handler registration
- `apps/backend/internal/gateway/websocket/*` - Hub, client, dispatcher
- `apps/backend/pkg/websocket/*` - Message types and handlers

Files to keep (for streaming data):
- Minimal WebSocket client for shell/process output
- Backend streaming handlers

---

### Phase 5: Authentication & Authorization

#### 5.1 JWT Authentication

**File**: `apps/backend/internal/auth/jwt.go`

```go
package auth

import (
    "time"

    "github.com/golang-jwt/jwt/v5"
)

type Claims struct {
    UserID      string   `json:"user_id"`
    Email       string   `json:"email"`
    Permissions []string `json:"permissions"`
    jwt.RegisteredClaims
}

func GenerateToken(userID, email string, secret []byte) (string, error) {
    claims := Claims{
        UserID: userID,
        Email:  email,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
        },
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString(secret)
}

func ValidateToken(tokenString string, secret []byte) (*Claims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
        return secret, nil
    })

    if err != nil {
        return nil, err
    }

    if claims, ok := token.Claims.(*Claims); ok && token.Valid {
        return claims, nil
    }

    return nil, jwt.ErrSignatureInvalid
}
```

**Middleware**: `apps/backend/internal/middleware/auth.go`

```go
func AuthMiddleware(secret []byte) gin.HandlerFunc {
    return func(c *gin.Context) {
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
            c.Abort()
            return
        }

        tokenString := strings.TrimPrefix(authHeader, "Bearer ")
        claims, err := auth.ValidateToken(tokenString, secret)
        if err != nil {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
            c.Abort()
            return
        }

        c.Set("user_id", claims.UserID)
        c.Set("user_email", claims.Email)
        c.Next()
    }
}
```

#### 5.2 Authorization in Electric Proxy

**File**: `apps/backend/internal/gateway/electric/authorization.go`

```go
func (p *Proxy) authorize(userID string, shape ShapeDefinition, params gin.Params) (bool, error) {
    // Example: Check if user has access to the workspace
    if workspaceID := params.ByName("workspace_id"); workspaceID != "" {
        return p.checkWorkspaceAccess(userID, workspaceID)
    }

    // Example: Check if user has access to the board
    if boardID := params.ByName("board_id"); boardID != "" {
        return p.checkBoardAccess(userID, boardID)
    }

    // Add more checks as needed
    return false, fmt.Errorf("unknown authorization scope")
}

func (p *Proxy) checkWorkspaceAccess(userID, workspaceID string) (bool, error) {
    // Query workspace_members table or similar
    var count int
    err := p.db.QueryRow(`
        SELECT COUNT(*) FROM workspace_members
        WHERE workspace_id = $1 AND user_id = $2
    `, workspaceID, userID).Scan(&count)

    return count > 0, err
}
```

---

### Phase 6: Deployment & Migration

#### 6.1 Development Environment

**Docker Compose**: Use the setup from Phase 1.1.2

**Start services**:
```bash
cd apps/backend
docker-compose up -d postgres electric
```

**Run migrations**:
```bash
# Using golang-migrate or similar
migrate -path ./migrations -database "postgres://postgres:postgres@localhost:5432/kandev?sslmode=disable" up
```

**Start backend**:
```bash
go run cmd/kandev/main.go
```

**Start frontend**:
```bash
cd apps/web
npm run dev
```

#### 6.2 Production Deployment

**Infrastructure requirements**:
1. PostgreSQL 14+ with logical replication enabled
2. Electric service (Docker or Kubernetes)
3. Backend API server
4. Frontend (static hosting or SSR)

**Example Kubernetes manifests**:

`postgres.yaml`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: postgres-config
data:
  wal_level: logical
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgres
spec:
  serviceName: postgres
  replicas: 1
  template:
    spec:
      containers:
      - name: postgres
        image: postgres:16
        env:
        - name: POSTGRES_DB
          value: kandev
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: password
        command:
        - postgres
        - -c
        - wal_level=logical
        volumeMounts:
        - name: postgres-data
          mountPath: /var/lib/postgresql/data
  volumeClaimTemplates:
  - metadata:
      name: postgres-data
    spec:
      accessModes: [ "ReadWriteOnce" ]
      resources:
        requests:
          storage: 10Gi
```

`electric.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: electric
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: electric
        image: electricsql/electric:latest
        env:
        - name: DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: electric-secret
              key: database-url
        - name: AUTH_MODE
          value: secure
        - name: ELECTRIC_FEATURE_FLAGS
          value: allow_subqueries,tagged_subqueries
        ports:
        - containerPort: 3000
---
apiVersion: v1
kind: Service
metadata:
  name: electric
spec:
  selector:
    app: electric
  ports:
  - port: 3000
    targetPort: 3000
```

#### 6.3 Migration Path

**Phase A: Parallel Run (Week 1-2)**
- Deploy Electric infrastructure alongside existing WebSocket system
- Add Electric shapes for a subset of data (e.g., tasks only)
- Feature flag to toggle between WebSocket and Electric for testing
- Monitor performance, latency, and error rates

**Phase B: Gradual Migration (Week 3-4)**
- Migrate feature by feature to Electric:
  1. Task management (tasks, workflow steps)
  2. Session messages
  3. Git tracking
  4. Workspaces and boards
- Keep WebSocket for streaming data
- Remove WebSocket handlers as features are migrated

**Phase C: Cleanup (Week 5)**
- Remove WebSocket hub, dispatcher, NATS event bus
- Remove deprecated code paths
- Update documentation
- Performance tuning and optimization

**Phase D: Monitoring (Ongoing)**
- Track Electric sync latency
- Monitor PostgreSQL replication lag
- Alert on connection failures or sync errors
- Optimize shape queries and indexes

---

## Performance Considerations

### Query Optimization

**Indexes for Electric Shapes**:
```sql
-- Workspace-scoped queries
CREATE INDEX idx_boards_workspace_id ON boards(workspace_id);
CREATE INDEX idx_tasks_board_id ON tasks(board_id);

-- Session-scoped queries
CREATE INDEX idx_session_messages_session_id ON task_session_messages(task_session_id);
CREATE INDEX idx_git_snapshots_session_id ON task_session_git_snapshots(session_id);
CREATE INDEX idx_commits_session_id ON task_session_commits(session_id);

-- User-scoped queries
CREATE INDEX idx_workspaces_user_id ON workspace_members(user_id);

-- Composite indexes for complex queries
CREATE INDEX idx_tasks_board_workflow ON tasks(board_id, workflow_step_id);
```

### Shape Scoping

**Best Practices**:
- Scope shapes by workspace/board/session to minimize data transfer
- Use WHERE clauses to filter server-side (never client-side)
- Avoid syncing entire tables; always scope to user's context
- Consider pagination for large datasets

**Example**:
```typescript
// BAD: Syncing all tasks
useShape(TASKS_SHAPE, {});

// GOOD: Syncing tasks for specific board
useShape(TASKS_SHAPE, { board_id: currentBoardId });
```

### Network Efficiency

**Electric automatically provides**:
- Incremental sync (only changed rows)
- Compression
- Connection pooling
- Automatic retries with backoff

**Additional optimizations**:
- Use `columns` parameter to fetch only needed columns
- Implement pagination for long lists
- Cache shape streams for 5 minutes to avoid recreation

---

## Security Considerations

### Server-Side Security

1. **Proxy Pattern**: All Electric requests go through backend proxy
2. **Server-Side WHERE Clauses**: Table and WHERE clause defined server-side
3. **Authorization Checks**: Validate user has access before proxying request
4. **Input Validation**: Sanitize all parameterized values
5. **Rate Limiting**: Implement rate limits on shape endpoints

### Client-Side Security

1. **Token Management**: Refresh tokens before expiry
2. **HTTPS Only**: Enforce HTTPS in production
3. **CSP Headers**: Configure Content Security Policy
4. **Secrets**: Never embed API keys or secrets in frontend

### PostgreSQL Security

1. **Dedicated Role**: Electric uses restricted `electric_sync` role
2. **Read-Only Access**: Electric role only has SELECT privileges
3. **Row-Level Security**: Consider RLS for additional protection
4. **Connection Limits**: Configure max connections for Electric role

---

## Testing Strategy

### Unit Tests

**Backend**:
- Test Electric proxy routing and parameter substitution
- Test authorization logic for different user scopes
- Test mutation endpoints with transaction ID generation

**Frontend**:
- Test shape hooks with mocked Electric responses
- Test mutation functions with optimistic updates
- Test error handling and retry logic

### Integration Tests

**End-to-End**:
- Test shape subscription and real-time updates
- Test mutations and Electric sync reconciliation
- Test offline mode and reconnection
- Test concurrent updates and conflict resolution

### Performance Tests

- Measure sync latency for different shape sizes
- Test concurrent connections (100, 1000, 10000 clients)
- Measure PostgreSQL replication lag under load
- Test Electric memory usage and CPU utilization

---

## Rollback Plan

If issues arise during migration:

1. **Feature Flag Rollback**: Toggle back to WebSocket via feature flag
2. **Database Rollback**: Keep SQLite as backup; migrations are reversible
3. **Infrastructure Rollback**: Keep WebSocket infrastructure until Phase C
4. **Monitoring**: Set up alerts for sync failures, high latency, errors

---

## Cost Analysis

### Current WebSocket Infrastructure

**Operational Costs**:
- Backend: Custom hub, dispatcher, NATS event bus (~2000 LOC)
- Frontend: WebSocket client, handlers, reconnection logic (~1500 LOC)
- Maintenance: Manual message routing, subscription management

### Electric SQL Infrastructure

**Operational Costs**:
- Electric service: Docker container (minimal resources)
- PostgreSQL: Logical replication (minimal overhead, ~1-5% CPU)
- Backend: Proxy routes (~500 LOC)
- Frontend: Shape hooks (~300 LOC)

**Benefits**:
- **Code reduction**: ~2500 LOC removed
- **Feature addition**: Offline-first, conflict resolution, automatic sync
- **Developer experience**: Less boilerplate, better types, local-first queries

---

## Timeline Estimate

| Phase | Duration | Key Deliverables |
|-------|----------|------------------|
| Phase 1: Infrastructure | 1-2 weeks | PostgreSQL setup, Electric service, Docker Compose |
| Phase 2: Proxy Implementation | 1 week | Backend proxy, shape definitions, frontend hooks |
| Phase 3: Mutation Handling | 1 week | Mutation endpoints, optimistic updates, txid tracking |
| Phase 4: Replace WebSocket | 2 weeks | Migrate all shapes, refactor contexts, remove deprecated code |
| Phase 5: Auth & Security | 1 week | JWT implementation, authorization logic, rate limiting |
| Phase 6: Deployment | 1-2 weeks | Kubernetes manifests, migration strategy, monitoring |
| **Total** | **7-9 weeks** | Full Electric SQL implementation |

---

## Conclusion

Migrating kandev from WebSockets to Electric SQL will:

1. **Simplify architecture** by removing custom synchronization infrastructure
2. **Improve developer experience** with type-safe, local-first data access
3. **Add offline-first capabilities** with automatic conflict resolution
4. **Enhance security** with server-side authorization and proxy pattern
5. **Reduce maintenance burden** by leveraging Electric's automatic sync

The vibe-kanban implementation provides a proven pattern that can be adapted to kandev's requirements. The hybrid approach (Electric for data, WebSocket for streaming) ensures we keep the best of both worlds.

**Recommendation**: Proceed with phased implementation, starting with infrastructure setup and a small subset of shapes (tasks only) to validate the approach before full migration.

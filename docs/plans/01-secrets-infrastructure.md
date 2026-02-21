# Plan 01: Secrets Infrastructure

> Encrypted secrets store for API keys, tokens, and sensitive configuration.
> Foundation for Sprites.dev API key, future SSH keys, and third-party service tokens.

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Master Key Management](#master-key-management)
4. [Encryption Scheme](#encryption-scheme)
5. [Backend Implementation](#backend-implementation)
6. [API Endpoints](#api-endpoints)
7. [Frontend Implementation](#frontend-implementation)
8. [Integration with Credential Manager](#integration-with-credential-manager)
9. [Migration & Rollout](#migration--rollout)
10. [Security Considerations](#security-considerations)

---

## Overview

Kandev needs a way to store user secrets (API keys, tokens) encrypted at rest. Currently, credentials are sourced from environment variables (`EnvProvider`) or a JSON file (`FileProvider`) — neither supports encrypted storage or user-managed secrets via the UI.

### Goals

- Store secrets encrypted at rest in SQLite using AES-256-GCM
- Master key file at `~/.kandev/master.key` (auto-generated, 0600 permissions)
- `SecretStore` interface abstractable to OS keyring in the future
- Integrate as a `CredentialProvider` in the existing credential manager chain
- API endpoints for CRUD (never return plaintext in list operations)
- Frontend settings page for managing secrets

### Non-Goals (This Phase)

- OS keyring integration (future)
- Secret rotation policies
- Multi-user / team secret sharing
- Hardware security module (HSM) support

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Frontend (Settings)                          │
│                                                                     │
│  ┌──────────────┐  ┌───────────────┐  ┌──────────────────────────┐ │
│  │ Secrets Page  │  │ Secret Form   │  │ Secret List (masked)     │ │
│  └──────┬───────┘  └───────┬───────┘  └──────────┬───────────────┘ │
│         │                  │                      │                 │
└─────────┼──────────────────┼──────────────────────┼─────────────────┘
          │ HTTP/WS          │                      │
          ▼                  ▼                      ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      Backend API Layer                               │
│                                                                     │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │              secrets/handlers/handlers.go                      │ │
│  │  POST /api/v1/secrets     → Create secret                     │ │
│  │  GET  /api/v1/secrets     → List secrets (names only)         │ │
│  │  GET  /api/v1/secrets/:id → Get secret metadata               │ │
│  │  PUT  /api/v1/secrets/:id → Update secret value               │ │
│  │  DELETE /api/v1/secrets/:id → Delete secret                   │ │
│  │  POST /api/v1/secrets/:id/reveal → Get plaintext (auth req)   │ │
│  └──────────────────────┬─────────────────────────────────────────┘ │
│                         │                                           │
│  ┌──────────────────────▼─────────────────────────────────────────┐ │
│  │              secrets/service.go                                │ │
│  │  SecretService — business logic, validation, categorization   │ │
│  └──────────────────────┬─────────────────────────────────────────┘ │
│                         │                                           │
│  ┌──────────────────────▼─────────────────────────────────────────┐ │
│  │              secrets/store.go (SecretStore interface)          │ │
│  │                                                               │ │
│  │  ┌─────────────────────────────────────────────────────────┐  │ │
│  │  │  SQLiteSecretStore (default)                            │  │ │
│  │  │  - Encrypts with AES-256-GCM before writing to DB      │  │ │
│  │  │  - Decrypts on read                                     │  │ │
│  │  │  - Uses master key from MasterKeyProvider               │  │ │
│  │  └─────────────────────────────────────────────────────────┘  │ │
│  │                                                               │ │
│  │  ┌─────────────────────────────────────────────────────────┐  │ │
│  │  │  KeyringSecretStore (future)                            │  │ │
│  │  │  - OS keyring via go-keyring                            │  │ │
│  │  └─────────────────────────────────────────────────────────┘  │ │
│  └──────────────────────┬─────────────────────────────────────────┘ │
│                         │                                           │
│  ┌──────────────────────▼─────────────────────────────────────────┐ │
│  │              secrets/crypto.go                                 │ │
│  │  MasterKeyProvider — loads/generates ~/.kandev/master.key     │ │
│  │  Encrypt(plaintext, key) → (ciphertext, nonce)                │ │
│  │  Decrypt(ciphertext, nonce, key) → plaintext                  │ │
│  └───────────────────────────────────────────────────────────────┘ │
│                         │                                           │
│  ┌──────────────────────▼─────────────────────────────────────────┐ │
│  │         credentials/secret_store_provider.go                  │ │
│  │  SecretStoreProvider implements CredentialProvider             │ │
│  │  - Bridges SecretStore → credential manager chain             │ │
│  └───────────────────────────────────────────────────────────────┘ │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        SQLite Database                               │
│                                                                     │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │  secrets table                                                │  │
│  │  ┌──────────┬────────┬───────────────┬───────┬─────────────┐ │  │
│  │  │ id (PK)  │ name   │ encrypted_val │ nonce │ category    │ │  │
│  │  │ TEXT     │ TEXT   │ BLOB          │ BLOB  │ TEXT        │ │  │
│  │  ├──────────┼────────┼───────────────┼───────┼─────────────┤ │  │
│  │  │ metadata │ env_key│ created_at    │       │ updated_at  │ │  │
│  │  │ TEXT/JSON│ TEXT   │ TIMESTAMP     │       │ TIMESTAMP   │ │  │
│  │  └──────────┴────────┴───────────────┴───────┴─────────────┘ │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Credential Resolution Chain

```
GetCredential("SPRITES_API_TOKEN")
    │
    ▼
┌────────────────────────────┐
│ 1. Check cache             │ ─── hit ──→ return cached
│    (in-memory)             │
└────────────┬───────────────┘
             │ miss
             ▼
┌────────────────────────────┐
│ 2. SecretStoreProvider     │ ─── found ──→ cache + return
│    (encrypted DB secrets)  │
└────────────┬───────────────┘
             │ not found
             ▼
┌────────────────────────────┐
│ 3. EnvProvider             │ ─── found ──→ cache + return
│    (environment variables) │
└────────────┬───────────────┘
             │ not found
             ▼
┌────────────────────────────┐
│ 4. FileProvider            │ ─── found ──→ cache + return
│    (JSON credentials file) │
└────────────┬───────────────┘
             │ not found
             ▼
        return error
```

---

## Master Key Management

### Key Generation & Storage

```
~/.kandev/
├── master.key          # 32-byte random key, 0600 permissions
├── data/               # SQLite database (existing)
└── ...
```

### Implementation: `secrets/crypto.go`

```go
package secrets

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "fmt"
    "io"
    "os"
    "path/filepath"
)

const (
    MasterKeyFile = "master.key"
    MasterKeySize = 32 // AES-256
)

// MasterKeyProvider manages the master encryption key.
type MasterKeyProvider struct {
    keyPath string
    key     []byte
}

// NewMasterKeyProvider creates a provider that loads or generates
// the master key from the given kandev config directory.
func NewMasterKeyProvider(kandevDir string) (*MasterKeyProvider, error) {
    keyPath := filepath.Join(kandevDir, MasterKeyFile)
    provider := &MasterKeyProvider{keyPath: keyPath}

    if err := provider.loadOrGenerate(); err != nil {
        return nil, fmt.Errorf("master key init: %w", err)
    }
    return provider, nil
}

func (p *MasterKeyProvider) loadOrGenerate() error {
    // Try to load existing key
    data, err := os.ReadFile(p.keyPath)
    if err == nil && len(data) == MasterKeySize {
        p.key = data
        return nil
    }

    // Generate new key
    key := make([]byte, MasterKeySize)
    if _, err := io.ReadFull(rand.Reader, key); err != nil {
        return fmt.Errorf("generate key: %w", err)
    }

    // Ensure directory exists
    if err := os.MkdirAll(filepath.Dir(p.keyPath), 0700); err != nil {
        return fmt.Errorf("create key dir: %w", err)
    }

    // Write with restrictive permissions
    if err := os.WriteFile(p.keyPath, key, 0600); err != nil {
        return fmt.Errorf("write key: %w", err)
    }

    p.key = key
    return nil
}

// Key returns the master key bytes. Never log this.
func (p *MasterKeyProvider) Key() []byte {
    return p.key
}
```

### Encryption Functions

```go
// Encrypt encrypts plaintext using AES-256-GCM with a random nonce.
// Returns (ciphertext, nonce, error).
func Encrypt(plaintext []byte, key []byte) ([]byte, []byte, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, nil, fmt.Errorf("create cipher: %w", err)
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, nil, fmt.Errorf("create GCM: %w", err)
    }

    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return nil, nil, fmt.Errorf("generate nonce: %w", err)
    }

    ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
    return ciphertext, nonce, nil
}

// Decrypt decrypts ciphertext using AES-256-GCM.
func Decrypt(ciphertext, nonce, key []byte) ([]byte, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, fmt.Errorf("create cipher: %w", err)
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, fmt.Errorf("create GCM: %w", err)
    }

    plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return nil, fmt.Errorf("decrypt: %w", err)
    }

    return plaintext, nil
}
```

---

## Encryption Scheme

### AES-256-GCM Details

```
┌──────────────────────────────────────────────────────────────┐
│                    Encryption Process                         │
│                                                              │
│  plaintext: "sk-ant-api03-abc..."                            │
│  master_key: [32 bytes from ~/.kandev/master.key]            │
│                                                              │
│  1. Generate random nonce (12 bytes)                         │
│  2. AES-256-GCM encrypt:                                     │
│     ciphertext = GCM.Seal(nonce, plaintext, master_key)      │
│  3. Store: (ciphertext, nonce) in DB                         │
│                                                              │
│  Decryption:                                                 │
│  1. Read (ciphertext, nonce) from DB                         │
│  2. plaintext = GCM.Open(nonce, ciphertext, master_key)      │
│                                                              │
│  Properties:                                                 │
│  - Authenticated encryption (integrity + confidentiality)    │
│  - Per-secret random nonce prevents ciphertext correlation   │
│  - 128-bit authentication tag                                │
│  - Master key never leaves memory / ~/.kandev/master.key     │
└──────────────────────────────────────────────────────────────┘
```

---

## Backend Implementation

### Package Structure

```
apps/backend/internal/secrets/
├── crypto.go          # MasterKeyProvider, Encrypt(), Decrypt()
├── models.go          # Secret model, categories, DTOs
├── store.go           # SecretStore interface
├── sqlite_store.go    # SQLite implementation
├── service.go         # Business logic, validation
├── handlers.go        # HTTP/WS handlers
└── provider.go        # CredentialProvider bridge (SecretStoreProvider)
```

### Models: `secrets/models.go`

```go
package secrets

import "time"

// SecretCategory groups secrets by purpose.
type SecretCategory string

const (
    CategoryAPIKey      SecretCategory = "api_key"      // LLM provider keys
    CategoryServiceToken SecretCategory = "service_token" // Sprites, exe.dev, etc.
    CategorySSHKey      SecretCategory = "ssh_key"       // SSH private keys
    CategoryCustom      SecretCategory = "custom"        // User-defined
)

// Secret represents a stored secret (decrypted in-memory representation).
type Secret struct {
    ID        string         `json:"id"`
    Name      string         `json:"name"`       // Human-readable name
    EnvKey    string         `json:"env_key"`     // Env var name (e.g., SPRITES_API_TOKEN)
    Category  SecretCategory `json:"category"`
    Metadata  map[string]string `json:"metadata,omitempty"` // e.g., {"service": "sprites.dev"}
    CreatedAt time.Time      `json:"created_at"`
    UpdatedAt time.Time      `json:"updated_at"`
    // Value is NEVER included in list/get responses. Only via Reveal().
}

// SecretWithValue is only used internally and for create/update operations.
type SecretWithValue struct {
    Secret
    Value string `json:"value,omitempty"`
}

// SecretListItem is returned by List() — never contains the value.
type SecretListItem struct {
    ID        string            `json:"id"`
    Name      string            `json:"name"`
    EnvKey    string            `json:"env_key"`
    Category  SecretCategory    `json:"category"`
    Metadata  map[string]string `json:"metadata,omitempty"`
    HasValue  bool              `json:"has_value"` // true if a value is stored
    CreatedAt time.Time         `json:"created_at"`
    UpdatedAt time.Time         `json:"updated_at"`
}
```

### Store Interface: `secrets/store.go`

```go
package secrets

import "context"

// SecretStore abstracts secret storage. Implementations handle
// encryption/decryption internally.
type SecretStore interface {
    // Create stores a new secret (encrypts the value).
    Create(ctx context.Context, secret *SecretWithValue) error

    // Get retrieves secret metadata (without value).
    Get(ctx context.Context, id string) (*Secret, error)

    // GetByEnvKey retrieves secret metadata by env key name.
    GetByEnvKey(ctx context.Context, envKey string) (*Secret, error)

    // Reveal retrieves the decrypted value of a secret.
    Reveal(ctx context.Context, id string) (string, error)

    // RevealByEnvKey retrieves the decrypted value by env key name.
    RevealByEnvKey(ctx context.Context, envKey string) (string, error)

    // Update updates a secret's value and/or metadata.
    Update(ctx context.Context, id string, secret *SecretWithValue) error

    // Delete permanently removes a secret.
    Delete(ctx context.Context, id string) error

    // List returns all secrets without values.
    List(ctx context.Context) ([]*SecretListItem, error)

    // ListByCategory returns secrets filtered by category.
    ListByCategory(ctx context.Context, category SecretCategory) ([]*SecretListItem, error)

    // Close releases resources.
    Close() error
}
```

### SQLite Implementation: `secrets/sqlite_store.go`

```go
// SQLiteSecretStore implements SecretStore with AES-256-GCM encryption.
type SQLiteSecretStore struct {
    db     *sqlx.DB  // writer
    ro     *sqlx.DB  // reader
    crypto *MasterKeyProvider
    logger *logger.Logger
}
```

**Database Schema:**

```sql
CREATE TABLE IF NOT EXISTS secrets (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    env_key        TEXT NOT NULL UNIQUE,
    encrypted_value BLOB NOT NULL,
    nonce          BLOB NOT NULL,
    category       TEXT NOT NULL DEFAULT 'custom',
    metadata       TEXT DEFAULT '{}',
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_secrets_env_key ON secrets(env_key);
CREATE INDEX IF NOT EXISTS idx_secrets_category ON secrets(category);
```

**Key operations:**

```go
func (s *SQLiteSecretStore) Create(ctx context.Context, secret *SecretWithValue) error {
    // 1. Validate: name and env_key not empty, env_key unique
    // 2. Encrypt: ciphertext, nonce, err := Encrypt([]byte(secret.Value), s.crypto.Key())
    // 3. INSERT INTO secrets (id, name, env_key, encrypted_value, nonce, category, metadata, ...)
}

func (s *SQLiteSecretStore) Reveal(ctx context.Context, id string) (string, error) {
    // 1. SELECT encrypted_value, nonce FROM secrets WHERE id = ?
    // 2. plaintext, err := Decrypt(ciphertext, nonce, s.crypto.Key())
    // 3. return string(plaintext), nil
}

func (s *SQLiteSecretStore) RevealByEnvKey(ctx context.Context, envKey string) (string, error) {
    // 1. SELECT encrypted_value, nonce FROM secrets WHERE env_key = ?
    // 2. Decrypt and return
}
```

### Credential Provider Bridge: `secrets/provider.go`

```go
package secrets

import (
    "context"
    "github.com/kandev/kandev/internal/agent/credentials"
)

// SecretStoreProvider bridges SecretStore into the credential provider chain.
// It implements credentials.CredentialProvider.
type SecretStoreProvider struct {
    store SecretStore
}

func NewSecretStoreProvider(store SecretStore) *SecretStoreProvider {
    return &SecretStoreProvider{store: store}
}

func (p *SecretStoreProvider) Name() string {
    return "secret_store"
}

func (p *SecretStoreProvider) GetCredential(ctx context.Context, key string) (*credentials.Credential, error) {
    value, err := p.store.RevealByEnvKey(ctx, key)
    if err != nil {
        return nil, err
    }
    return &credentials.Credential{
        Key:    key,
        Value:  value,
        Source: "secret_store",
    }, nil
}

func (p *SecretStoreProvider) ListAvailable(ctx context.Context) ([]string, error) {
    items, err := p.store.List(ctx)
    if err != nil {
        return nil, err
    }
    keys := make([]string, len(items))
    for i, item := range items {
        keys[i] = item.EnvKey
    }
    return keys, nil
}
```

### Service: `secrets/service.go`

```go
type Service struct {
    store  SecretStore
    logger *logger.Logger
}

// Validation rules:
// - Name: 1-100 chars, unique
// - EnvKey: uppercase, underscores, unique (e.g., SPRITES_API_TOKEN)
// - Value: 1-10000 chars
// - Category: must be valid SecretCategory
```

### Handlers: `secrets/handlers.go`

```go
type Handler struct {
    service *Service
    logger  *logger.Logger
}

func (h *Handler) RegisterRoutes(r chi.Router) {
    r.Route("/api/v1/secrets", func(r chi.Router) {
        r.Post("/", h.CreateSecret)
        r.Get("/", h.ListSecrets)
        r.Get("/{id}", h.GetSecret)
        r.Put("/{id}", h.UpdateSecret)
        r.Delete("/{id}", h.DeleteSecret)
        r.Post("/{id}/reveal", h.RevealSecret) // POST to avoid caching
    })
}

func (h *Handler) RegisterWSActions(dispatcher ws.Dispatcher) {
    dispatcher.Register("secrets.list", h.WSListSecrets)
    dispatcher.Register("secrets.create", h.WSCreateSecret)
    dispatcher.Register("secrets.update", h.WSUpdateSecret)
    dispatcher.Register("secrets.delete", h.WSDeleteSecret)
    dispatcher.Register("secrets.reveal", h.WSRevealSecret)
}
```

### Startup Wiring: `cmd/kandev/storage.go` additions

```go
// In provideRepositories():

// Initialize master key provider
kandevDir := filepath.Join(os.Getenv("HOME"), ".kandev")
masterKeyProvider, err := secrets.NewMasterKeyProvider(kandevDir)
if err != nil {
    return nil, fmt.Errorf("master key: %w", err)
}

// Create secret store
secretStore, cleanupSecrets, err := secrets.ProvideSQLiteStore(writer, reader, masterKeyProvider, log)
if err != nil {
    return nil, fmt.Errorf("secret store: %w", err)
}

// In provideLifecycleManager() — register as credential provider:
secretProvider := secrets.NewSecretStoreProvider(secretStore)
credsMgr.AddProvider(secretProvider) // First in chain (highest priority)
credsMgr.AddProvider(credentials.NewEnvProvider("KANDEV_"))
// ... rest of providers
```

---

## API Endpoints

### HTTP Endpoints

| Method | Path | Request Body | Response | Description |
|--------|------|-------------|----------|-------------|
| `POST` | `/api/v1/secrets` | `{name, env_key, value, category?, metadata?}` | `{id, name, env_key, category, metadata, created_at}` | Create secret |
| `GET` | `/api/v1/secrets` | — | `[{id, name, env_key, category, has_value, ...}]` | List (no values) |
| `GET` | `/api/v1/secrets/:id` | — | `{id, name, env_key, category, metadata, ...}` | Get metadata |
| `PUT` | `/api/v1/secrets/:id` | `{name?, value?, category?, metadata?}` | `{id, name, env_key, ...}` | Update |
| `DELETE` | `/api/v1/secrets/:id` | — | `204 No Content` | Delete |
| `POST` | `/api/v1/secrets/:id/reveal` | — | `{value}` | Get plaintext |

### WebSocket Actions

| Action | Payload | Response | Description |
|--------|---------|----------|-------------|
| `secrets.list` | `{}` | `[SecretListItem]` | List secrets |
| `secrets.create` | `{name, env_key, value, category?}` | `SecretListItem` | Create |
| `secrets.update` | `{id, name?, value?, category?}` | `SecretListItem` | Update |
| `secrets.delete` | `{id}` | `{success: true}` | Delete |
| `secrets.reveal` | `{id}` | `{value}` | Get plaintext |

### Request/Response DTOs

```go
// Create
type CreateSecretRequest struct {
    Name     string            `json:"name" validate:"required,min=1,max=100"`
    EnvKey   string            `json:"env_key" validate:"required,env_key_format"`
    Value    string            `json:"value" validate:"required,min=1,max=10000"`
    Category SecretCategory    `json:"category,omitempty"`
    Metadata map[string]string `json:"metadata,omitempty"`
}

type CreateSecretResponse struct {
    ID        string            `json:"id"`
    Name      string            `json:"name"`
    EnvKey    string            `json:"env_key"`
    Category  SecretCategory    `json:"category"`
    Metadata  map[string]string `json:"metadata,omitempty"`
    CreatedAt time.Time         `json:"created_at"`
}

// Reveal (POST to avoid URL caching of sensitive data)
type RevealSecretResponse struct {
    Value string `json:"value"`
}
```

---

## Frontend Implementation

### New Files

```
apps/web/
├── app/settings/secrets/
│   └── page.tsx                    # Secrets settings page route
├── components/settings/
│   ├── secrets-page.tsx            # Secrets management page component
│   └── secret-form-dialog.tsx      # Create/edit secret dialog
├── lib/api/domains/
│   └── secrets-api.ts              # API client for secrets endpoints
├── lib/types/
│   └── http-secrets.ts             # TypeScript types for secrets
├── lib/ws/handlers/
│   └── secrets.ts                  # WS event handlers
└── hooks/domains/settings/
    └── use-secrets.ts              # Secrets data hook
```

### TypeScript Types: `lib/types/http-secrets.ts`

```typescript
export type SecretCategory = 'api_key' | 'service_token' | 'ssh_key' | 'custom';

export interface SecretListItem {
  id: string;
  name: string;
  env_key: string;
  category: SecretCategory;
  metadata?: Record<string, string>;
  has_value: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateSecretRequest {
  name: string;
  env_key: string;
  value: string;
  category?: SecretCategory;
  metadata?: Record<string, string>;
}

export interface UpdateSecretRequest {
  name?: string;
  value?: string;
  category?: SecretCategory;
  metadata?: Record<string, string>;
}
```

### API Client: `lib/api/domains/secrets-api.ts`

```typescript
export const secretsApi = {
  list: () => fetchApi<SecretListItem[]>('/api/v1/secrets'),
  create: (req: CreateSecretRequest) => fetchApi<SecretListItem>('/api/v1/secrets', { method: 'POST', body: req }),
  update: (id: string, req: UpdateSecretRequest) => fetchApi<SecretListItem>(`/api/v1/secrets/${id}`, { method: 'PUT', body: req }),
  delete: (id: string) => fetchApi<void>(`/api/v1/secrets/${id}`, { method: 'DELETE' }),
  reveal: (id: string) => fetchApi<{ value: string }>(`/api/v1/secrets/${id}/reveal`, { method: 'POST' }),
};
```

### Store Slice Addition: `lib/state/slices/settings/settings-slice.ts`

```typescript
// Add to SettingsSlice state:
secrets: {
  items: SecretListItem[];
  loading: boolean;
  loaded: boolean;
}

// Add actions:
setSecrets: (items: SecretListItem[]) => void;
addSecret: (item: SecretListItem) => void;
updateSecret: (id: string, item: Partial<SecretListItem>) => void;
removeSecret: (id: string) => void;
```

### Secrets Page Component: `components/settings/secrets-page.tsx`

```
┌─────────────────────────────────────────────────────────────┐
│  Secrets                                          [+ Add]   │
│─────────────────────────────────────────────────────────────│
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │ 🔑 Anthropic API Key                                  │  │
│  │    ANTHROPIC_API_KEY  •  api_key  •  ●●●●●●●●●●●●    │  │
│  │                                    [Reveal] [Delete]  │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │ 🔑 Sprites API Token                                  │  │
│  │    SPRITES_API_TOKEN  •  service_token  •  ●●●●●●●●  │  │
│  │                                    [Reveal] [Delete]  │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │ 🔑 GitHub Token                                       │  │
│  │    GITHUB_TOKEN  •  service_token  •  ●●●●●●●●●●●●  │  │
│  │                                    [Reveal] [Delete]  │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

Components:
- Uses `@kandev/ui/card`, `@kandev/ui/button`, `@kandev/ui/dialog`, `@kandev/ui/badge`
- Secret values shown as masked dots, with a "Reveal" button that calls the reveal endpoint
- Create/Edit dialog with form validation (name, env_key format, value textarea)
- Delete with confirmation dialog
- Category badges with color coding

### Settings Sidebar Addition

Add "Secrets" item to the settings sidebar navigation, under a new "Security" section:

```typescript
// In settings-app-sidebar.tsx
{
  title: "Security",
  items: [
    { title: "Secrets", url: "/settings/secrets", icon: IconKey }
  ]
}
```

---

## Integration with Credential Manager

### Provider Registration Order

```go
// In provideLifecycleManager() or startup wiring:
credsMgr := credentials.NewManager(log)

// 1. Secret store (highest priority — user-managed secrets)
credsMgr.AddProvider(secrets.NewSecretStoreProvider(secretStore))

// 2. Environment variables (existing behavior)
credsMgr.AddProvider(credentials.NewEnvProvider("KANDEV_"))

// 3. Augment session provider (existing)
credsMgr.AddProvider(credentials.NewAugmentSessionProvider())

// 4. File provider (existing, lowest priority)
if credsFile := os.Getenv("KANDEV_CREDENTIALS_FILE"); credsFile != "" {
    credsMgr.AddProvider(credentials.NewFileProvider(credsFile))
}
```

### Usage Example: Sprites API Token

```go
// When creating a Sprites executor instance:
token, err := credsMgr.GetCredentialValue(ctx, "SPRITES_API_TOKEN")
if err != nil {
    return fmt.Errorf("sprites API token not configured: %w", err)
}
client := sprites.New(token)
```

---

## Migration & Rollout

### Database Migration

The `secrets` table is created via `initSchema()` in the SQLite secret store (same pattern as other repos). No migration files needed — the table is created if it doesn't exist.

### Master Key Bootstrapping

```
First startup after secrets feature:
1. Check ~/.kandev/master.key exists
2. If not, generate 32 random bytes, write with 0600 perms
3. Log: "Master key generated at ~/.kandev/master.key"
4. Warning: "Back up your master key — losing it means losing all stored secrets"

Subsequent startups:
1. Load ~/.kandev/master.key
2. Validate 32 bytes
3. Initialize SQLiteSecretStore with loaded key
```

### Backward Compatibility

- Existing `EnvProvider` and `FileProvider` continue to work unchanged
- `SecretStoreProvider` is additive — no breaking changes to credential resolution
- Users can gradually migrate from env vars to stored secrets

---

## Security Considerations

### Threat Model

| Threat | Mitigation |
|--------|-----------|
| DB file stolen | Secrets encrypted with AES-256-GCM |
| Master key stolen | File permissions 0600; future: OS keyring |
| Memory dump | Secrets in memory only during active use; cache cleared on shutdown |
| API response leak | List endpoints never return values; reveal is POST |
| Log injection | Secret values never logged (zap fields skip value) |
| SQL injection | Parameterized queries via sqlx |

### Future: OS Keyring Migration

```go
// KeyringSecretStore implements SecretStore using the OS keyring.
// When implemented, secrets can be migrated:
// 1. Read all secrets from SQLiteSecretStore (decrypt with master key)
// 2. Write each to KeyringSecretStore
// 3. Delete from SQLite
// 4. Update config to use keyring backend
```

### Master Key Backup Warning

On first generation, the backend logs a prominent warning and the frontend shows a one-time notification:

```
⚠️  Master encryption key generated at ~/.kandev/master.key
    Back up this file — losing it means losing access to all stored secrets.
    The key is NOT recoverable.
```

---

## Files Changed Summary

### New Files
| File | Description |
|------|-------------|
| `apps/backend/internal/secrets/crypto.go` | MasterKeyProvider, Encrypt/Decrypt |
| `apps/backend/internal/secrets/models.go` | Secret, SecretListItem, categories |
| `apps/backend/internal/secrets/store.go` | SecretStore interface |
| `apps/backend/internal/secrets/sqlite_store.go` | SQLite + encryption implementation |
| `apps/backend/internal/secrets/service.go` | Validation, business logic |
| `apps/backend/internal/secrets/handlers.go` | HTTP + WS handlers |
| `apps/backend/internal/secrets/provider.go` | SecretStoreProvider (CredentialProvider bridge) |
| `apps/web/app/settings/secrets/page.tsx` | Route page |
| `apps/web/components/settings/secrets-page.tsx` | Secrets management UI |
| `apps/web/components/settings/secret-form-dialog.tsx` | Create/edit dialog |
| `apps/web/lib/api/domains/secrets-api.ts` | API client |
| `apps/web/lib/types/http-secrets.ts` | TypeScript types |
| `apps/web/lib/ws/handlers/secrets.ts` | WS event handlers |
| `apps/web/hooks/domains/settings/use-secrets.ts` | Data hook |

### Modified Files
| File | Change |
|------|--------|
| `apps/backend/cmd/kandev/storage.go` | Init MasterKeyProvider + SecretStore |
| `apps/backend/cmd/kandev/agents.go` | Register SecretStoreProvider in credential chain |
| `apps/backend/cmd/kandev/routes.go` | Register secrets HTTP routes |
| `apps/backend/cmd/kandev/websocket.go` | Register secrets WS actions |
| `apps/web/lib/state/slices/settings/settings-slice.ts` | Add secrets state |
| `apps/web/components/settings/settings-app-sidebar.tsx` | Add Secrets nav item |

---

*Last updated: 2026-02-20*

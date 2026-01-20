package mcpconfig

type ServerType string

type ServerMode string

const (
	ServerTypeStdio          ServerType = "stdio"
	ServerTypeHTTP           ServerType = "http"
	ServerTypeSSE            ServerType = "sse"
	ServerTypeStreamableHTTP ServerType = "streamable_http"
)

const (
	ServerModeAuto       ServerMode = "auto"
	ServerModeShared     ServerMode = "shared"
	ServerModePerSession ServerMode = "per_session"
)

type ServerDef struct {
	Type    ServerType        `json:"type,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Mode    ServerMode        `json:"mode,omitempty"`
	Meta    map[string]any    `json:"meta,omitempty"`
	Extra   map[string]any    `json:"extra,omitempty"`
}

type AgentConfig struct {
	AgentID   string               `json:"agent_id"`
	AgentName string               `json:"agent_name"`
	Enabled   bool                 `json:"enabled"`
	Servers   map[string]ServerDef `json:"servers"`
	Meta      map[string]any       `json:"meta,omitempty"`
}

type ResolvedServer struct {
	Name    string
	Type    ServerType
	Mode    ServerMode
	Command string
	Args    []string
	Env     map[string]string
	URL     string
	Headers map[string]string
}

// Policy controls which MCP transports are allowed and how they should be rewritten.
type Policy struct {
	AllowStdio          bool
	AllowHTTP           bool
	AllowSSE            bool
	AllowStreamableHTTP bool
	URLRewrite          map[string]string
	EnvInjection        map[string]string
}

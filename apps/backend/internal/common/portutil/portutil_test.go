package portutil

import (
	"strings"
	"testing"
)

func TestAllocatePort(t *testing.T) {
	port, err := AllocatePort()
	if err != nil {
		t.Fatalf("AllocatePort() failed: %v", err)
	}

	if port <= 0 || port > 65535 {
		t.Errorf("AllocatePort() returned invalid port: %d", port)
	}

	t.Logf("Allocated port: %d", port)
}

func TestAllocatePortUniqueness(t *testing.T) {
	// Allocate multiple ports and ensure they're different
	ports := make(map[int]bool)
	for i := 0; i < 10; i++ {
		port, err := AllocatePort()
		if err != nil {
			t.Fatalf("AllocatePort() failed on iteration %d: %v", i, err)
		}
		if ports[port] {
			t.Errorf("AllocatePort() returned duplicate port: %d", port)
		}
		ports[port] = true
	}
}

func TestFindUniquePlaceholders(t *testing.T) {
	tests := []struct {
		name     string
		command  string
		expected []string
	}{
		{
			name:     "single $PORT",
			command:  "npm run dev -- --port $PORT",
			expected: []string{"PORT"},
		},
		{
			name:     "single ${PORT}",
			command:  "vite --port ${PORT}",
			expected: []string{"PORT"},
		},
		{
			name:     "mixed $PORT and ${PORT}",
			command:  "start --port $PORT --host localhost:${PORT}",
			expected: []string{"PORT"},
		},
		{
			name:     "multiple different placeholders",
			command:  "start --api-port $API_PORT --web-port $WEB_PORT",
			expected: []string{"API_PORT", "WEB_PORT"},
		},
		{
			name:     "no placeholders",
			command:  "npm run dev",
			expected: []string{},
		},
		{
			name:     "placeholder at start",
			command:  "$PORT npm run dev",
			expected: []string{"PORT"},
		},
		{
			name:     "placeholder at end",
			command:  "npm run dev $PORT",
			expected: []string{"PORT"},
		},
		{
			name:     "underscore prefix",
			command:  "npm run dev -- --port $_PORT",
			expected: []string{"_PORT"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findUniquePlaceholders(tt.command)

			if len(result) != len(tt.expected) {
				t.Errorf("findUniquePlaceholders() returned %d placeholders, expected %d. Got: %v, Expected: %v",
					len(result), len(tt.expected), result, tt.expected)
				return
			}

			// Convert to map for easier comparison (order doesn't matter)
			resultMap := make(map[string]bool)
			for _, p := range result {
				resultMap[p] = true
			}

			for _, expected := range tt.expected {
				if !resultMap[expected] {
					t.Errorf("findUniquePlaceholders() missing expected placeholder: %s. Got: %v",
						expected, result)
				}
			}
		})
	}
}

func TestTransformCommand(t *testing.T) {
	tests := []struct {
		name            string
		command         string
		expectError     bool
		validateCommand func(string) bool
		validateEnv     func(map[string]string) bool
	}{
		{
			name:        "simple $PORT replacement",
			command:     "npm run dev -- --port $PORT",
			expectError: false,
			validateCommand: func(cmd string) bool {
				// Should not contain $PORT anymore
				if strings.Contains(cmd, "$PORT") {
					return false
				}
				// Should start with the same prefix
				if !strings.HasPrefix(cmd, "npm run dev -- --port ") {
					return false
				}
				return true
			},
			validateEnv: func(env map[string]string) bool {
				portStr, ok := env["PORT"]
				if !ok {
					return false
				}
				// Should be a valid port number string
				return len(portStr) > 0 && portStr != "0"
			},
		},
		{
			name:        "${PORT} replacement",
			command:     "vite --port ${PORT}",
			expectError: false,
			validateCommand: func(cmd string) bool {
				if strings.Contains(cmd, "${PORT}") || strings.Contains(cmd, "$PORT") {
					return false
				}
				if !strings.HasPrefix(cmd, "vite --port ") {
					return false
				}
				return true
			},
			validateEnv: func(env map[string]string) bool {
				_, ok := env["PORT"]
				return ok
			},
		},
		{
			name:        "no placeholders",
			command:     "npm run dev",
			expectError: false,
			validateCommand: func(cmd string) bool {
				return cmd == "npm run dev"
			},
			validateEnv: func(env map[string]string) bool {
				return len(env) == 0
			},
		},
		{
			name:        "multiple same placeholders",
			command:     "start --port $PORT --callback-port $PORT",
			expectError: false,
			validateCommand: func(cmd string) bool {
				if strings.Contains(cmd, "$PORT") {
					return false
				}
				// Both instances should be replaced with the same port
				parts := strings.Fields(cmd)
				if len(parts) != 5 {
					return false
				}
				// Extract the two port values
				port1 := parts[2] // After --port
				port2 := parts[4] // After --callback-port
				return port1 == port2 && port1 != ""
			},
			validateEnv: func(env map[string]string) bool {
				_, ok := env["PORT"]
				return ok && len(env) == 1
			},
		},
		{
			name:        "multiple different placeholders",
			command:     "start --api $API_PORT --web $WEB_PORT",
			expectError: false,
			validateCommand: func(cmd string) bool {
				if strings.Contains(cmd, "$API_PORT") || strings.Contains(cmd, "$WEB_PORT") {
					return false
				}
				return strings.HasPrefix(cmd, "start --api ")
			},
			validateEnv: func(env map[string]string) bool {
				_, hasAPI := env["API_PORT"]
				_, hasWeb := env["WEB_PORT"]
				return hasAPI && hasWeb && len(env) == 2
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformedCmd, env, err := TransformCommand(tt.command)

			if tt.expectError && err == nil {
				t.Error("TransformCommand() expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("TransformCommand() unexpected error: %v", err)
				return
			}

			if err == nil {
				if !tt.validateCommand(transformedCmd) {
					t.Errorf("TransformCommand() produced invalid command: %s", transformedCmd)
				}

				if !tt.validateEnv(env) {
					t.Errorf("TransformCommand() produced invalid env: %v", env)
				}

				t.Logf("Original: %s", tt.command)
				t.Logf("Transformed: %s", transformedCmd)
				t.Logf("Env: %v", env)
			}
		})
	}
}

func TestTransformCommandPortInEnv(t *testing.T) {
	// Test that the port in the command matches the port in the env
	command := "npm run dev -- --port $PORT"
	transformedCmd, env, err := TransformCommand(command)

	if err != nil {
		t.Fatalf("TransformCommand() failed: %v", err)
	}

	portStr, ok := env["PORT"]
	if !ok {
		t.Fatal("TransformCommand() did not set PORT in env")
	}

	if !strings.Contains(transformedCmd, portStr) {
		t.Errorf("TransformCommand() command does not contain the allocated port. Command: %s, Port: %s",
			transformedCmd, portStr)
	}
}

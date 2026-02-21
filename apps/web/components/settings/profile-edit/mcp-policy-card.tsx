"use client";

import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";

function parseMcpPolicyJson(currentPolicy: string | undefined): Record<string, unknown> {
  try {
    if (currentPolicy?.trim()) {
      return JSON.parse(currentPolicy) as Record<string, unknown>;
    }
  } catch {
    // ignore
  }
  return {};
}

function McpPresetButton({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      className="cursor-pointer rounded-full border border-muted-foreground/30 px-2 py-1 text-xs hover:bg-muted"
      onClick={onClick}
    >
      {label}
    </button>
  );
}

export function validateMcpPolicy(value: string | undefined): string | null {
  const raw = value ?? "";
  if (!raw.trim()) return null;
  try {
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed))
      return "MCP policy must be a JSON object";
  } catch {
    return "Invalid JSON";
  }
  return null;
}

type McpPolicyCardProps = {
  mcpPolicy: string;
  mcpPolicyError: string | null;
  onPolicyChange: (value: string) => void;
};

export function McpPolicyCard({ mcpPolicy, mcpPolicyError, onPolicyChange }: McpPolicyCardProps) {
  const applyPreset = (updater: (parsed: Record<string, unknown>) => Record<string, unknown>) => {
    const parsed = parseMcpPolicyJson(mcpPolicy);
    const next = updater(parsed);
    onPolicyChange(JSON.stringify(next, null, 2));
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          MCP Policy
          <span className="rounded-full border border-muted-foreground/30 px-2 py-0.5 text-[10px] uppercase tracking-wide text-muted-foreground">
            Advanced
          </span>
        </CardTitle>
        <CardDescription>JSON policy overrides for MCP servers on this profile.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        <Label htmlFor="mcp-policy">MCP policy JSON</Label>
        <Textarea
          id="mcp-policy"
          value={mcpPolicy}
          onChange={(event) => onPolicyChange(event.target.value)}
          placeholder='{"allow_stdio":true,"allow_http":true}'
          rows={8}
        />
        {mcpPolicyError && <p className="text-xs text-destructive">{mcpPolicyError}</p>}
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-xs font-medium text-muted-foreground">Quick presets</p>
          <McpPresetButton
            label="Only HTTP/SSE"
            onClick={() =>
              applyPreset((p) => ({ ...p, allow_stdio: false, allow_http: true, allow_sse: true }))
            }
          />
          <McpPresetButton
            label="Only stdio"
            onClick={() =>
              applyPreset((p) => ({ ...p, allow_stdio: true, allow_http: false, allow_sse: false }))
            }
          />
          <McpPresetButton
            label="Allowlist GitHub + Playwright"
            onClick={() =>
              applyPreset((p) => {
                const existing = Array.isArray(p.allowlist_servers)
                  ? (p.allowlist_servers as string[])
                  : [];
                return {
                  ...p,
                  allowlist_servers: Array.from(new Set([...existing, "github", "playwright"])),
                };
              })
            }
          />
          <McpPresetButton
            label="Rewrite localhost for Docker"
            onClick={() =>
              applyPreset((p) => {
                const existing =
                  p.url_rewrite && typeof p.url_rewrite === "object"
                    ? (p.url_rewrite as Record<string, string>)
                    : {};
                return {
                  ...p,
                  url_rewrite: {
                    ...existing,
                    "http://localhost:3000": "http://host.docker.internal:3000",
                  },
                };
              })
            }
          />
        </div>
      </CardContent>
    </Card>
  );
}

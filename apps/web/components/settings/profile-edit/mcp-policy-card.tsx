"use client";

import { CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { SettingsCard } from "@/components/settings/settings-card";
import { useTranslation } from "react-i18next";

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
  baselinePolicy?: string;
  mcpPolicyError: string | null;
  onPolicyChange: (value: string) => void;
};

export function McpPolicyCard({
  mcpPolicy,
  baselinePolicy,
  mcpPolicyError,
  onPolicyChange,
}: McpPolicyCardProps) {
  const { t } = useTranslation();
  const isDirty = baselinePolicy !== undefined && mcpPolicy !== baselinePolicy;
  const applyPreset = (updater: (parsed: Record<string, unknown>) => Record<string, unknown>) => {
    const parsed = parseMcpPolicyJson(mcpPolicy);
    const next = updater(parsed);
    onPolicyChange(JSON.stringify(next, null, 2));
  };

  return (
    <SettingsCard isDirty={isDirty}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {t("executors:mcpPolicy")}
          <span className="rounded-full border border-muted-foreground/30 px-2 py-0.5 text-[10px] uppercase tracking-wide text-muted-foreground">
            {t("executors:advanced")}
          </span>
        </CardTitle>
        <CardDescription>{t("executors:mcpPolicyOverridesForProfile")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        <Label htmlFor="mcp-policy">{t("executors:mcpPolicyJson")}</Label>
        <Textarea
          id="mcp-policy"
          value={mcpPolicy}
          onChange={(event) => onPolicyChange(event.target.value)}
          placeholder='{"allow_stdio":true,"allow_http":true}'
          rows={8}
          data-settings-dirty={isDirty}
        />
        {mcpPolicyError && <p className="text-xs text-destructive">{mcpPolicyError}</p>}
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-xs font-medium text-muted-foreground">{t("executors:quickPresets")}</p>
          <McpPresetButton
            label={t("executors:onlyHttpSse")}
            onClick={() =>
              applyPreset((p) => ({ ...p, allow_stdio: false, allow_http: true, allow_sse: true }))
            }
          />
          <McpPresetButton
            label={t("executors:onlyStdio")}
            onClick={() =>
              applyPreset((p) => ({ ...p, allow_stdio: true, allow_http: false, allow_sse: false }))
            }
          />
          <McpPresetButton
            label={t("executors:allowlistGithubPlaywright")}
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
            label={t("executors:rewriteLocalhostForDocker")}
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
    </SettingsCard>
  );
}

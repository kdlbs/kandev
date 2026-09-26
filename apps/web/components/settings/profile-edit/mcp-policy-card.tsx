"use client";

import { CardContent } from "@kandev/ui/card";
import { Textarea } from "@kandev/ui/textarea";
import { SettingsCard } from "@/components/settings/settings-card";
import { useTranslation } from "react-i18next";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import {
  SETTINGS_TYPOGRAPHY,
  SettingsErrorText,
  SettingsFieldLabel,
} from "@/components/settings/settings-typography";

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

// Returns a catalog key (or null when valid) so the message resolves at render
// rather than at module load — the same contract as the local validator in
// app/settings/executor/[id]/page.tsx.
export function validateMcpPolicy(value: string | undefined): string | null {
  const raw = value ?? "";
  if (!raw.trim()) return null;
  try {
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed))
      return "executors:mcpPolicyMustBeAJsonObject";
  } catch {
    return "executors:invalidJson";
  }
  return null;
}

type McpPolicyCardProps = {
  mcpPolicy: string;
  baselinePolicy?: string;
  mcpPolicyErrorKey: string | null;
  onPolicyChange: (value: string) => void;
  discoveryTargetId?: string;
};

export function McpPolicyCard({
  mcpPolicy,
  baselinePolicy,
  mcpPolicyErrorKey,
  onPolicyChange,
  discoveryTargetId,
}: McpPolicyCardProps) {
  const { t } = useTranslation();
  const isDirty = baselinePolicy !== undefined && mcpPolicy !== baselinePolicy;
  const applyPreset = (updater: (parsed: Record<string, unknown>) => Record<string, unknown>) => {
    const parsed = parseMcpPolicyJson(mcpPolicy);
    const next = updater(parsed);
    onPolicyChange(JSON.stringify(next, null, 2));
  };

  return (
    <SettingsCard isDirty={isDirty} discoveryTargetId={discoveryTargetId}>
      <SettingsCardHeader
        title={
          <span className="flex items-center gap-2">
            {t("executors:mcpPolicy")}
            <span
              className={
                "rounded-full border border-muted-foreground/30 px-2 py-0.5 uppercase tracking-wide " +
                SETTINGS_TYPOGRAPHY.meta
              }
            >
              {t("executors:advanced")}
            </span>
          </span>
        }
        description={t("executors:mcpPolicyOverridesForProfile")}
      />
      <CardContent className="space-y-2">
        <SettingsFieldLabel htmlFor="mcp-policy">{t("executors:mcpPolicyJson")}</SettingsFieldLabel>
        <Textarea
          id="mcp-policy"
          value={mcpPolicy}
          onChange={(event) => onPolicyChange(event.target.value)}
          placeholder='{"allow_stdio":true,"allow_http":true}'
          rows={8}
          data-settings-dirty={isDirty}
        />
        {mcpPolicyErrorKey && <SettingsErrorText>{t(mcpPolicyErrorKey)}</SettingsErrorText>}
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

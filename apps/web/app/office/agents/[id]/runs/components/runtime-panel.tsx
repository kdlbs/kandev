"use client";

import type { RunRuntimeDetail } from "@/lib/api/domains/office-extended-api";

type Props = {
  runtime: RunRuntimeDetail;
};

export function RuntimePanel({ runtime }: Props) {
  const capabilityKeys = enabledCapabilityKeys(runtime.capabilities);
  const hasSnapshot = Object.keys(runtime.input_snapshot ?? {}).length > 0;
  const hasRuntime =
    capabilityKeys.length > 0 || hasSnapshot || runtime.skills.length > 0 || runtime.session_id;
  if (!hasRuntime) return null;

  return (
    <div className="rounded-lg border border-border p-4 space-y-3" data-testid="runtime-panel">
      <div>
        <h2 className="text-sm font-semibold">Runtime</h2>
        {runtime.session_id && (
          <p className="text-xs text-muted-foreground font-mono mt-1">{runtime.session_id}</p>
        )}
      </div>
      {capabilityKeys.length > 0 && (
        <div className="space-y-2">
          <div className="text-xs text-muted-foreground uppercase tracking-wider">Capabilities</div>
          <div className="flex flex-wrap gap-1.5" data-testid="runtime-capabilities">
            {capabilityKeys.map((key) => (
              <span
                key={key}
                className="rounded border border-border px-2 py-0.5 text-xs font-mono"
              >
                {key}
              </span>
            ))}
          </div>
        </div>
      )}
      {runtime.skills.length > 0 && (
        <div className="space-y-2">
          <div className="text-xs text-muted-foreground uppercase tracking-wider">Skills</div>
          <div className="space-y-1" data-testid="runtime-skills">
            {runtime.skills.map((skill) => (
              <div key={skill.skill_id} className="text-xs font-mono break-all">
                {skill.skill_id}{" "}
                {skill.version && <span className="text-muted-foreground">v{skill.version}</span>}
                {skill.content_hash && (
                  <span className="text-muted-foreground">
                    {" "}
                    hash {shortHash(skill.content_hash)}
                  </span>
                )}
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function shortHash(hash: string): string {
  return hash.length > 12 ? hash.slice(0, 12) : hash;
}

function enabledCapabilityKeys(capabilities: Record<string, unknown>): string[] {
  return Object.entries(capabilities ?? {})
    .filter(([, value]) => value === true)
    .map(([key]) => key)
    .sort();
}

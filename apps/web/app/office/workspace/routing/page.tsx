"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { IconDeviceFloppy } from "@tabler/icons-react";
import { toast } from "sonner";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import { useWorkspaceRouting } from "@/hooks/domains/office/use-workspace-routing";
import { useProviderHealth } from "@/hooks/domains/office/use-provider-health";
import { useRoutingPreview } from "@/hooks/domains/office/use-routing-preview";
import type {
  ProviderProfile,
  Tier,
  TierPerReason,
  WorkspaceRouting,
} from "@/lib/state/slices/office/types";
import {
  AgentPreviewTable,
  DefaultTierSelector,
  ProviderHealthBanner,
  ProviderOrderEditor,
  ProviderTierMapping,
  RoutingEnableCard,
  WakeReasonTierCard,
} from "./components";

const DEFAULT_PROFILE: ProviderProfile = { tier_map: {} };

function emptyConfig(): WorkspaceRouting {
  return {
    enabled: false,
    provider_order: [],
    default_tier: "balanced",
    provider_profiles: {},
    tier_per_reason: {},
  };
}

export default function ProviderRoutingPage() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const routing = useWorkspaceRouting(workspaceId);
  const health = useProviderHealth(workspaceId);
  const preview = useRoutingPreview(workspaceId);

  const [draft, setDraft] = useState<WorkspaceRouting | null>(null);
  const [saving, setSaving] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<string[]>([]);

  useEffect(() => {
    if (routing.config) setDraft(routing.config);
    else if (!routing.isLoading && routing.config === undefined) setDraft(emptyConfig());
  }, [routing.config, routing.isLoading]);

  const dirty = useMemo(
    () => draft && JSON.stringify(draft) !== JSON.stringify(routing.config ?? emptyConfig()),
    [draft, routing.config],
  );

  const onSave = useCallback(async () => {
    if (!draft || !workspaceId) return;
    setSaving(true);
    setFieldErrors([]);
    try {
      await routing.update(draft);
      void preview.refresh();
      toast.success("Routing settings saved");
    } catch (err) {
      const errs = extractValidationDetails(err);
      if (errs.length > 0) setFieldErrors(errs);
      toast.error(err instanceof Error ? err.message : "Failed to save");
    } finally {
      setSaving(false);
    }
  }, [draft, workspaceId, routing, preview]);

  if (!workspaceId || !draft) {
    return <div className="p-6 text-sm text-muted-foreground">Loading…</div>;
  }

  return (
    <PageBody
      draft={draft}
      setDraft={setDraft}
      knownProviders={routing.knownProviders}
      saving={saving}
      dirty={!!dirty}
      fieldErrors={fieldErrors}
      onSave={onSave}
      health={health.health}
      onRetry={routing.retry}
      previewAgents={preview.agents}
      previewLoading={preview.isLoading}
    />
  );
}

type PageBodyProps = {
  draft: WorkspaceRouting;
  setDraft: (cfg: WorkspaceRouting) => void;
  knownProviders: string[];
  saving: boolean;
  dirty: boolean;
  fieldErrors: string[];
  onSave: () => Promise<void>;
  health: ReturnType<typeof useProviderHealth>["health"];
  onRetry: ReturnType<typeof useWorkspaceRouting>["retry"];
  previewAgents: ReturnType<typeof useRoutingPreview>["agents"];
  previewLoading: boolean;
};

function PageBody({
  draft,
  setDraft,
  knownProviders,
  saving,
  dirty,
  fieldErrors,
  onSave,
  health,
  onRetry,
  previewAgents,
  previewLoading,
}: PageBodyProps) {
  const setEnabled = (v: boolean) => setDraft({ ...draft, enabled: v });
  const setTier = (t: Tier) => setDraft({ ...draft, default_tier: t });
  const setTierPerReason = (m: TierPerReason) => setDraft({ ...draft, tier_per_reason: m });
  const setOrder = (next: string[]) => {
    const profiles = { ...draft.provider_profiles };
    for (const p of next) {
      if (!profiles[p]) profiles[p] = { ...DEFAULT_PROFILE };
    }
    setDraft({ ...draft, provider_order: next, provider_profiles: profiles });
  };
  const setProfile = (p: string, prof: ProviderProfile) =>
    setDraft({ ...draft, provider_profiles: { ...draft.provider_profiles, [p]: prof } });

  const sectionsDisabled = !draft.enabled;

  return (
    <div className="max-w-4xl mx-auto p-6 space-y-4">
      <div>
        <h1 className="text-lg font-semibold">Provider routing</h1>
        <p className="text-sm text-muted-foreground">
          Map Office agents to providers and tiers with controlled fallback. Advanced; off by
          default.
        </p>
      </div>

      <RoutingEnableCard enabled={draft.enabled} onChange={setEnabled} disabled={saving} />

      {draft.enabled && (
        <>
          <DefaultTierSelector
            value={draft.default_tier}
            onChange={setTier}
            disabled={sectionsDisabled || saving}
          />
          <WakeReasonTierCard
            config={draft}
            value={draft.tier_per_reason ?? {}}
            onChange={setTierPerReason}
            disabled={sectionsDisabled || saving}
          />
          <ProviderOrderEditor
            order={draft.provider_order}
            knownProviders={knownProviders}
            onChange={setOrder}
            disabled={sectionsDisabled || saving}
          />
          {draft.provider_order.map((pid) => (
            <ProviderTierMapping
              key={pid}
              providerId={pid}
              profile={draft.provider_profiles[pid] ?? DEFAULT_PROFILE}
              defaultTier={draft.default_tier}
              onChange={(prof) => setProfile(pid, prof)}
              disabled={saving}
            />
          ))}
        </>
      )}

      <AgentPreviewTable agents={previewAgents} isLoading={previewLoading} />

      <ProviderHealthBanner health={health} onRetry={onRetry} />

      {fieldErrors.length > 0 && (
        <ul className="rounded-lg border border-destructive bg-destructive/10 p-3 text-xs space-y-1">
          {fieldErrors.map((m, i) => (
            <li key={i}>{m}</li>
          ))}
        </ul>
      )}

      {dirty && (
        <div className="flex justify-end">
          <Button onClick={onSave} disabled={saving} className="cursor-pointer gap-1.5">
            <IconDeviceFloppy className="h-4 w-4" />
            {saving ? "Saving…" : "Save"}
          </Button>
        </div>
      )}
    </div>
  );
}

function extractValidationDetails(err: unknown): string[] {
  if (!(err instanceof ApiError)) return [];
  const body = err.body;
  if (!body || typeof body !== "object") return [];
  const out: string[] = [];
  const obj = body as {
    error?: string;
    field?: string;
    details?: Array<{ provider_id?: string; field?: string; message?: string }>;
  };
  if (obj.error && obj.field) out.push(`${obj.field}: ${obj.error}`);
  if (Array.isArray(obj.details)) {
    for (const d of obj.details) {
      if (d?.message) {
        const prefix = d.provider_id ? `${d.provider_id}: ` : "";
        out.push(`${prefix}${d.message}`);
      }
    }
  }
  return out;
}

"use client";

import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useAppStore } from "@/components/state-provider";
import { useWorkspaceRouting } from "@/hooks/domains/office/use-workspace-routing";
import { useProviderHealth } from "@/hooks/domains/office/use-provider-health";
import { useRoutingPreview } from "@/hooks/domains/office/use-routing-preview";
import type {
  ProviderProfile,
  ExecutionProfileSummary,
  RoleMeta,
  RoleTierMap,
  Tier,
  TierPerReason,
  WorkspaceRouting,
} from "@/lib/state/slices/office/types";
import {
  AuthorExclusionStepsCard,
  AgentPreviewTable,
  DefaultTierSelector,
  ProviderHealthBanner,
  ProviderOrderEditor,
  ProviderTierMapping,
  RoleTierCard,
  RoutingEnableCard,
  WakeReasonTierCard,
} from "./components";
import { useTranslation } from "react-i18next";
import { AuthorExclusionWarningList } from "@/components/author-exclusion-warning-list";
import type { AuthorExclusionWarning } from "@/lib/types/author-exclusion";
import { providerLabel } from "./components/provider-order-editor";
import { useAuthorExclusionDraft } from "./use-author-exclusion-draft";
import { useRoutingSave } from "./use-routing-save";

const DEFAULT_PROFILE: ProviderProfile = { tier_map: {} };

// Stable fallback: a Zustand selector that returns a fresh `[]` on every call
// (when `office.meta` hasn't loaded yet) breaks useSyncExternalStore's
// reference-equality snapshot check and infinite-loops into React error #185
// ("Maximum update depth exceeded"), caught by the route error boundary. See
// EMPTY_PREVIEW / EMPTY_HEALTH in the sibling hooks for the same pattern.
const EMPTY_ROLES: RoleMeta[] = [];

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
  const { t } = useTranslation();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const roles = useAppStore((s) => s.office.meta?.roles ?? EMPTY_ROLES);
  const routing = useWorkspaceRouting(workspaceId);
  const health = useProviderHealth(workspaceId);
  const preview = useRoutingPreview(workspaceId);

  const [draft, setDraft] = useState<WorkspaceRouting | null>(null);

  useEffect(() => {
    if (routing.config) setDraft(routing.config);
    else if (!routing.isLoading && routing.config === undefined) setDraft(emptyConfig());
  }, [routing.config, routing.isLoading]);

  const saved = useMemo(() => routing.config ?? emptyConfig(), [routing.config]);
  const exclusion = useAuthorExclusionDraft(routing.authorExclusionSteps);
  const { saving, fieldErrors } = useRoutingSave({
    draft,
    saved,
    exclusionChanges: exclusion.changes,
    currentWarnings: routing.authorExclusionWarnings,
    setDraft,
    resetExclusion: exclusion.reset,
    updateRouting: routing.update,
    updateAuthorExclusion: routing.updateAuthorExclusion,
    onSaved: () => void preview.refresh(),
  });

  if (!workspaceId || !draft) {
    return <div className="p-6 text-sm text-muted-foreground">{t("common:loading")}</div>;
  }

  return (
    <PageBody
      draft={draft}
      setDraft={setDraft}
      roles={roles}
      knownProviders={routing.knownProviders}
      executionProfiles={routing.executionProfiles}
      saving={saving}
      fieldErrors={fieldErrors}
      health={health.health}
      onRetry={routing.retry}
      previewAgents={preview.agents}
      previewLoading={preview.isLoading}
      authorExclusionWarnings={routing.authorExclusionWarnings}
      exclusionSteps={
        <AuthorExclusionStepsCard
          steps={routing.authorExclusionSteps}
          draft={exclusion.draft}
          onChange={exclusion.setValue}
          disabled={saving}
        />
      }
    />
  );
}

type PageBodyProps = {
  draft: WorkspaceRouting;
  setDraft: (cfg: WorkspaceRouting) => void;
  roles: RoleMeta[];
  knownProviders: string[];
  executionProfiles: ExecutionProfileSummary[];
  saving: boolean;
  fieldErrors: string[];
  health: ReturnType<typeof useProviderHealth>["health"];
  onRetry: ReturnType<typeof useWorkspaceRouting>["retry"];
  previewAgents: ReturnType<typeof useRoutingPreview>["agents"];
  previewLoading: boolean;
  authorExclusionWarnings: AuthorExclusionWarning[];
  exclusionSteps: ReactNode;
};

function PageBody({
  draft,
  setDraft,
  roles,
  knownProviders,
  executionProfiles,
  saving,
  fieldErrors,
  health,
  onRetry,
  previewAgents,
  previewLoading,
  authorExclusionWarnings,
  exclusionSteps,
}: PageBodyProps) {
  const { t } = useTranslation();
  const setEnabled = (v: boolean) => setDraft({ ...draft, enabled: v });
  // Named `tier`, not `t`: a parameter called `t` shadows the translate function.
  const setTier = (tier: Tier) => setDraft({ ...draft, default_tier: tier });
  const setTierPerReason = (m: TierPerReason) => setDraft({ ...draft, tier_per_reason: m });
  const setRoleTiers = (m: RoleTierMap) => setDraft({ ...draft, role_tiers: m });
  const setOrder = (next: string[]) => {
    const profiles = { ...draft.provider_profiles };
    for (const p of next) {
      if (!profiles[p]) profiles[p] = { ...DEFAULT_PROFILE };
    }
    setDraft({ ...draft, provider_order: next, provider_profiles: profiles });
  };
  const setProfile = (p: string, prof: ProviderProfile) =>
    setDraft({ ...draft, provider_profiles: { ...draft.provider_profiles, [p]: prof } });

  const sectionsDisabled = saving;

  return (
    <div className="max-w-4xl mx-auto p-6 pb-32 space-y-4">
      <div>
        <h1 className="text-lg font-semibold">{t("office:providerRoutingHeading")}</h1>
        <p className="text-sm text-muted-foreground">{t("office:mapOfficeAgentsToProvidersAnd")}</p>
      </div>

      <AuthorExclusionWarningList
        warnings={authorExclusionWarnings}
        formatProvider={providerLabel}
        surface="routing"
      />

      <RoutingEnableCard enabled={draft.enabled} onChange={setEnabled} disabled={saving} />

      <DefaultTierSelector
        value={draft.default_tier}
        onChange={setTier}
        disabled={sectionsDisabled}
      />
      <RoleTierCard
        roles={roles}
        config={draft}
        value={draft.role_tiers ?? {}}
        onChange={setRoleTiers}
        disabled={sectionsDisabled}
      />
      <WakeReasonTierCard
        config={draft}
        value={draft.tier_per_reason ?? {}}
        onChange={setTierPerReason}
        disabled={sectionsDisabled}
      />
      <ProviderOrderEditor
        order={draft.provider_order}
        knownProviders={knownProviders}
        onChange={setOrder}
        disabled={sectionsDisabled}
      />
      {draft.provider_order.map((pid) => (
        <ProviderTierMapping
          key={pid}
          providerId={pid}
          profile={draft.provider_profiles[pid] ?? DEFAULT_PROFILE}
          executionProfiles={executionProfiles}
          defaultTier={draft.default_tier}
          onChange={(prof) => setProfile(pid, prof)}
          disabled={saving}
        />
      ))}

      {exclusionSteps}

      <AgentPreviewTable agents={previewAgents} isLoading={previewLoading} />

      <ProviderHealthBanner health={health} onRetry={onRetry} />

      {fieldErrors.length > 0 && (
        <ul className="rounded-lg border border-destructive bg-destructive/10 p-3 text-xs space-y-1">
          {fieldErrors.map((m, i) => (
            <li key={i}>{m}</li>
          ))}
        </ul>
      )}
    </div>
  );
}

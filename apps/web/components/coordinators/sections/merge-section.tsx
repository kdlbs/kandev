"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { Skeleton } from "@kandev/ui/skeleton";
import type { ControlSetting } from "@/lib/api/domains/coordinator-api";
import type {
  Enforcement,
  MergeRepo,
  ProtectionObservation,
} from "@/lib/api/domains/coordinator-merge-api";
import {
  useMergeSettings,
  type MergeSaveError,
} from "@/hooks/domains/coordinator/use-merge-settings";
import {
  effectivePolicy,
  hasUnknownProtection,
  isPrincipalInactive,
  MERGE_POLICIES,
  protectionOf,
  type MergeDraft,
} from "@/lib/coordinators/merge-draft";
import { formatDateTime } from "@/lib/i18n/formats";

const POLICY_LABEL_KEY: Record<ControlSetting, string> = {
  denied: "coordinator:mergePolicyDenied",
  requires_approval: "coordinator:mergePolicyRequiresApproval",
  automatic: "coordinator:mergePolicyAutomatic",
};

const ENFORCEMENT_KEY: Record<Enforcement, string> = {
  enforced: "coordinator:mergeEnforced",
  not_enforced: "coordinator:mergeNotEnforced",
  unknown: "coordinator:mergeUnknown",
};

const SAVE_ERROR_KEY: Record<MergeSaveError, string> = {
  conflict: "coordinator:mergeSaveConflict",
  principal_valid: "coordinator:mergePrincipalValid",
  merge_unavailable: "coordinator:mergeSaveUnavailable",
  repository_unknown: "coordinator:mergeSaveRepositoryUnknown",
  failed: "coordinator:mergeSaveFailed",
};

function Statements() {
  const { t } = useTranslation();
  return (
    <ul
      className="text-muted-foreground list-disc space-y-1 pl-5 text-sm"
      data-testid="merge-statements"
    >
      <li>{t("coordinator:mergeStatementDefault")}</li>
      <li>{t("coordinator:mergeStatementDrift")}</li>
      <li>{t("coordinator:mergeStatementNoChecks")}</li>
      <li>{t("coordinator:mergeStatementQueue")}</li>
    </ul>
  );
}

function ProtectionRow({ observation }: { observation: ProtectionObservation }) {
  const { t } = useTranslation();
  const cells: [string, Enforcement][] = [
    ["coordinator:mergeProtectionPipeline", observation.pipeline],
    ["coordinator:mergeProtectionApprovals", observation.approvals],
    ["coordinator:mergeProtectionThreads", observation.threads],
  ];
  return (
    <li
      className="space-y-1 py-2 text-sm"
      data-testid={`merge-protection-${observation.repository_id}-${observation.branch}`}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="font-medium break-all">{observation.branch}</span>
        <span className="text-muted-foreground text-xs">
          {t("coordinator:mergeProtectionObserved", {
            time: formatDateTime(observation.observed_at),
          })}
        </span>
      </div>
      <div className="flex flex-wrap gap-x-4 gap-y-1">
        {cells.map(([labelKey, value]) => (
          <span key={labelKey} data-testid="merge-protection-cell" data-value={value}>
            {t(labelKey)}: {t(ENFORCEMENT_KEY[value])}
          </span>
        ))}
      </div>
    </li>
  );
}

type ProtectionProps = {
  repo: MergeRepo;
  observations: ProtectionObservation[];
  refreshing: boolean;
  onRetry: () => void;
};

function Protection({ repo, observations, refreshing, onRetry }: ProtectionProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1" data-testid={`merge-protection-${repo.repository_id}`}>
      <h4 className="text-muted-foreground text-xs font-medium">
        {t("coordinator:mergeProtectionHeading")}
      </h4>
      {observations.length === 0 ? (
        <p className="text-muted-foreground text-xs">{t("coordinator:mergeProtectionNone")}</p>
      ) : (
        <ul className="divide-border divide-y">
          {observations.map((o) => (
            <ProtectionRow key={o.branch} observation={o} />
          ))}
        </ul>
      )}
      {hasUnknownProtection(observations) && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="min-h-11 w-full cursor-pointer sm:min-h-0 sm:w-auto"
          disabled={refreshing}
          onClick={onRetry}
          data-testid={`merge-protection-retry-${repo.repository_id}`}
        >
          {t("coordinator:retry")}
        </Button>
      )}
    </div>
  );
}

function PolicyChoice({
  repo,
  value,
  disabled,
  onChange,
}: {
  repo: MergeRepo;
  value: ControlSetting;
  disabled: boolean;
  onChange: (policy: ControlSetting) => void;
}) {
  const { t } = useTranslation();
  return (
    <RadioGroup
      value={value}
      onValueChange={(next) => onChange(next as ControlSetting)}
      disabled={disabled}
      className="flex flex-col gap-1 sm:flex-row sm:flex-wrap sm:gap-4"
      aria-label={t("coordinator:mergePolicyLabel", { name: repo.name })}
      data-testid={`merge-policy-${repo.repository_id}`}
    >
      {MERGE_POLICIES.map((policy) => (
        <label
          key={policy}
          className="flex min-h-11 cursor-pointer items-center gap-2 text-sm sm:min-h-0"
        >
          <RadioGroupItem value={policy} id={`merge-${repo.repository_id}-${policy}`} />
          {t(POLICY_LABEL_KEY[policy])}
        </label>
      ))}
    </RadioGroup>
  );
}

type RepoRowProps = {
  repo: MergeRepo;
  draft: MergeDraft;
  canManage: boolean;
  observations: ProtectionObservation[];
  refreshing: boolean;
  onPolicy: (policy: ControlSetting) => void;
  onReplace: () => void;
  onRetryProtection: () => void;
};

function PrincipalNotice({
  repo,
  draft,
  canManage,
  onReplace,
}: Pick<RepoRowProps, "repo" | "draft" | "canManage" | "onReplace">) {
  const { t } = useTranslation();
  if (!isPrincipalInactive(repo)) return null;
  const marked = draft[repo.repository_id]?.replacePrincipal === true;
  return (
    <div
      className="border-destructive/40 space-y-2 rounded-md border p-3 text-sm"
      data-testid={`merge-principal-${repo.repository_id}`}
    >
      <p>{t("coordinator:mergePrincipalInactive")}</p>
      {canManage && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="min-h-11 w-full cursor-pointer sm:min-h-0 sm:w-auto"
          disabled={marked}
          onClick={onReplace}
          data-testid={`merge-replace-${repo.repository_id}`}
        >
          {marked ? t("coordinator:mergeReplaceOnSave") : t("coordinator:mergeReplacePrincipal")}
        </Button>
      )}
    </div>
  );
}

function RepoRow(props: RepoRowProps) {
  const { repo, draft, canManage, observations } = props;
  return (
    <li
      className="space-y-3 py-4 md:grid md:grid-cols-[minmax(0,14rem)_minmax(0,1fr)] md:gap-x-6 md:space-y-0"
      data-testid={`merge-repo-${repo.repository_id}`}
    >
      <span className="text-sm font-medium break-words" data-testid="merge-repo-name">
        {repo.name}
      </span>
      <div className="space-y-3">
        <PolicyChoice
          repo={repo}
          value={effectivePolicy(repo, draft)}
          disabled={!canManage}
          onChange={props.onPolicy}
        />
        <PrincipalNotice
          repo={repo}
          draft={draft}
          canManage={canManage}
          onReplace={props.onReplace}
        />
        <Protection
          repo={repo}
          observations={observations}
          refreshing={props.refreshing}
          onRetry={props.onRetryProtection}
        />
      </div>
    </li>
  );
}

type Props = { workspaceId: string; coordinatorId: string; canManage: boolean };

/** The Merge section: repository policy, provider protection and the explanatory statements. */
export function MergeSection({ workspaceId, coordinatorId, canManage }: Props) {
  const { t } = useTranslation();
  const merge = useMergeSettings({ workspaceId, coordinatorId, canManage, enabled: true });

  if (merge.status === "error") {
    return (
      <div className="space-y-2" data-testid="merge-load-error">
        <p className="text-destructive text-sm">{t("coordinator:loadError")}</p>
        <Button
          type="button"
          variant="outline"
          className="min-h-11 cursor-pointer sm:min-h-0"
          onClick={merge.retry}
        >
          {t("coordinator:retry")}
        </Button>
      </div>
    );
  }
  if (merge.status === "loading") {
    return <Skeleton className="h-4 w-64" data-testid="merge-loading" />;
  }
  if (merge.status === "off") return null;

  return (
    <div className="space-y-4" data-testid="merge-section">
      <Statements />
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="min-h-11 w-full cursor-pointer sm:min-h-0 sm:w-auto"
          disabled={merge.refreshing}
          onClick={() => void merge.refreshProtection()}
          data-testid="merge-protection-refresh"
        >
          {t("coordinator:mergeProtectionRefresh")}
        </Button>
        {merge.refreshFailed && (
          <p role="alert" className="text-destructive text-sm" data-testid="merge-refresh-failed">
            {t("coordinator:mergeProtectionRefreshFailed")}
          </p>
        )}
      </div>
      {merge.repos.length === 0 ? (
        <p className="text-muted-foreground text-sm">{t("coordinator:mergeRepositoriesNone")}</p>
      ) : (
        <ul className="divide-border divide-y" data-testid="merge-repositories">
          {merge.repos.map((repo) => (
            <RepoRow
              key={repo.repository_id}
              repo={repo}
              draft={merge.draft}
              canManage={canManage}
              observations={protectionOf(merge.protection, repo.repository_id)}
              refreshing={merge.refreshing}
              onPolicy={(policy) => merge.choosePolicy(repo, policy)}
              onReplace={() => merge.replacePrincipal(repo)}
              onRetryProtection={() => void merge.refreshProtection()}
            />
          ))}
        </ul>
      )}
      {merge.saveError && (
        <p role="alert" className="text-destructive text-sm" data-testid="merge-save-error">
          {t(SAVE_ERROR_KEY[merge.saveError])}
        </p>
      )}
    </div>
  );
}

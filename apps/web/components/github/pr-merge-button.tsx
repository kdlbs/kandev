"use client";

import { useState } from "react";
import { IconChevronDown, IconGitMerge } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useTaskWorkspaceId } from "@/components/task/task-workspace-context";
import { useToast } from "@/components/toast-provider";
import { useRepoMergeAccess } from "@/hooks/domains/github/use-repo-merge-access";
import { useRepoMergeMethods } from "@/hooks/domains/github/use-repo-merge-methods";
import { mergePR } from "@/lib/api/domains/github-api";
import { getGitHubActorLabel } from "@/lib/github-auth";
import type { MergeMethod, TaskPR } from "@/lib/types/github";
import { canAttemptPRMerge } from "./pr-task-icon";
import { PRMergeAccessNotice } from "./pr-merge-access-notice";
import { useTranslation } from "react-i18next";

function MutationActor({ actor }: { actor: string | null }) {
  const { t } = useTranslation();
  if (!actor) return null;
  return (
    <span className="text-[11px] text-muted-foreground">{t("github:mergeAsActor", { actor })}</span>
  );
}

function CompactMergeButton({
  label,
  disabled,
  actor,
  onPrimaryClick,
  extraMethods,
  onPickMethod,
}: Omit<MergeButtonShellProps, "compact">) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-1.5">
      <button
        type="button"
        data-testid="pr-merge-button"
        onClick={onPrimaryClick}
        disabled={disabled}
        className="inline-flex w-full items-center justify-center gap-1.5 rounded-md bg-green-600 px-2 py-1.5 text-xs font-medium text-white shadow-sm transition-colors hover:bg-green-700 disabled:cursor-default disabled:opacity-60 cursor-pointer"
      >
        <IconGitMerge className="h-3.5 w-3.5" />
        {label}
      </button>
      {extraMethods.length > 0 && (
        <div className="flex items-center justify-center gap-1 text-[11px] text-muted-foreground">
          <span>{t("github:or")}</span>
          {extraMethods.map((method) => (
            <button
              key={method}
              type="button"
              data-testid={`pr-merge-method-${method}`}
              disabled={disabled}
              onClick={(event) => {
                event.stopPropagation();
                onPickMethod(method);
              }}
              className="cursor-pointer rounded px-1.5 py-0.5 font-medium hover:bg-muted hover:text-foreground disabled:cursor-default disabled:opacity-60"
            >
              {t(mergeShortLabelKey(method))}
            </button>
          ))}
        </div>
      )}
      <MutationActor actor={actor} />
    </div>
  );
}

// Readiness and the current actor's target-repository access both gate manual merge.
export function PRMergeButton({
  taskPR,
  onMerged,
  compact = false,
}: {
  taskPR: TaskPR;
  onMerged?: () => void;
  compact?: boolean;
}) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [merging, setMerging] = useState(false);
  // After GitHub accepts a direct or queued merge, stay hidden until refreshed
  // provider state catches up so a repeat click cannot submit a second request.
  const [acceptedFor, setAcceptedFor] = useState<string | null>(null);
  const workspaceId = useTaskWorkspaceId(taskPR.task_id);
  const methods = useRepoMergeMethods(workspaceId, taskPR.owner, taskPR.repo, taskPR.github_host);

  // Reset after switching PRs or after GitHub reports an observable lifecycle
  // or mergeability transition, including a PR ejected from a merge queue.
  const signature = `${taskPR.id}:${taskPR.state}:${taskPR.mergeable_state}`;
  const accepted = acceptedFor === signature;
  const ready = canAttemptPRMerge(taskPR);
  const access = useRepoMergeAccess(
    workspaceId,
    taskPR.owner,
    taskPR.repo,
    ready && !accepted,
    taskPR.github_host ?? "github.com",
  );
  const mutationActor = getGitHubActorLabel(access.actor);

  if (accepted || !ready) return null;
  if (access.state !== "allowed") {
    return (
      <PRMergeAccessNotice
        state={access.state}
        actor={mutationActor}
        repository={`${taskPR.owner}/${taskPR.repo}`}
        onRetry={access.refresh}
        compact={compact}
      />
    );
  }
  // After access is confirmed, missing method metadata uses the backend resolver.
  const allowed = allowedMethods(methods);
  const primary = pickPrimaryMethod(allowed);

  const runMerge = async (method?: MergeMethod) => {
    if (!workspaceId || merging || access.state !== "allowed") return;
    const submittedSignature = signature;
    setMerging(true);
    try {
      const result = await mergePR(workspaceId, taskPR.owner, taskPR.repo, taskPR.pr_number, {
        mergeMethod: method,
        expectedHost: taskPR.github_host ?? "github.com",
      });
      setAcceptedFor(submittedSignature);
      toast({
        description: t(
          result.status === "queued" ? "github:prAddedToMergeQueue" : "github:prMerged",
        ),
        variant: "success",
      });
      onMerged?.();
    } catch (err) {
      access.refresh();
      toast({
        title: t("github:failedToMerge"),
        description: err instanceof Error ? err.message : t("github:anErrorOccurred"),
        variant: "error",
      });
    } finally {
      setMerging(false);
    }
  };

  const handlePrimary = (e: React.MouseEvent) => {
    e.stopPropagation();
    void runMerge(primary);
  };

  return (
    <MergeButtonShell
      compact={compact}
      label={
        merging
          ? t("github:merging")
          : t(
              taskPR.mergeable_state === "blocked"
                ? "github:mergeMethodDefault"
                : mergeLabelKey(primary),
            )
      }
      disabled={merging}
      actor={mutationActor}
      onPrimaryClick={handlePrimary}
      extraMethods={
        taskPR.mergeable_state === "blocked" ? [] : allowed.filter((m) => m !== primary)
      }
      onPickMethod={(m) => void runMerge(m)}
    />
  );
}

type MergeButtonShellProps = {
  compact: boolean;
  label: string;
  disabled: boolean;
  actor: string | null;
  onPrimaryClick: (e: React.MouseEvent) => void;
  extraMethods: MergeMethod[];
  onPickMethod: (m: MergeMethod) => void;
};

function MergeButtonShell({
  compact,
  label,
  disabled,
  actor,
  onPrimaryClick,
  extraMethods,
  onPickMethod,
}: MergeButtonShellProps) {
  const { t } = useTranslation();
  // Compact (hover popover): a single full-width primary button plus quiet
  // "or <method>" links for the alternates. Deliberately avoids a dropdown —
  // its menu renders in a detached portal that closes the hover popover the
  // moment the pointer leaves it (the bug this replaces). Plain buttons keep
  // every action inside the popover's own DOM.
  if (compact) {
    return (
      <CompactMergeButton
        label={label}
        disabled={disabled}
        actor={actor}
        onPrimaryClick={onPrimaryClick}
        extraMethods={extraMethods}
        onPickMethod={onPickMethod}
      />
    );
  }

  const showDropdown = extraMethods.length > 0;
  const primaryBtn = (
    <Button
      data-testid="pr-merge-button"
      className={controlSizingClassName(
        "standard",
        `cursor-pointer gap-1.5 border-0 bg-green-600 text-white hover:bg-green-700 dark:bg-green-600 dark:hover:bg-green-500 ${showDropdown ? "rounded-r-none" : ""}`,
      )}
      onClick={onPrimaryClick}
      disabled={disabled}
    >
      <IconGitMerge className="h-3.5 w-3.5" />
      {label}
    </Button>
  );

  if (!showDropdown) {
    return (
      <span className="flex flex-col items-end gap-1 self-end">
        {primaryBtn}
        <MutationActor actor={actor} />
      </span>
    );
  }

  return (
    <span className="flex flex-col items-end gap-1 self-end">
      <span className="inline-flex">
        {primaryBtn}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              data-testid="pr-merge-button-more"
              aria-label={t("github:chooseMergeMethod")}
              disabled={disabled}
              onClick={(e) => e.stopPropagation()}
              className="inline-flex items-center rounded-r-md border-l border-green-700/40 bg-green-600 px-2 text-white hover:bg-green-700 dark:bg-green-600 dark:hover:bg-green-500 disabled:opacity-60 cursor-pointer"
            >
              <IconChevronDown className="h-3.5 w-3.5" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-auto">
            {extraMethods.map((m) => (
              <DropdownMenuItem key={m} onSelect={() => onPickMethod(m)}>
                {t(mergeLabelKey(m))}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </span>
      <MutationActor actor={actor} />
    </span>
  );
}

/** `method` is GitHub's persisted merge method; only the label is copy. */
function mergeLabelKey(method?: MergeMethod): string {
  switch (method) {
    case "squash":
      return "github:mergeMethodSquash";
    case "rebase":
      return "github:mergeMethodRebase";
    case "merge":
      return "github:mergeMethodMergeCommit";
    default:
      return "github:mergeMethodDefault";
  }
}

// Short form used by the compact "or <method>" alternates so the row fits the
// narrow popover without wrapping.
function mergeShortLabelKey(method: MergeMethod): string {
  switch (method) {
    case "squash":
      return "github:mergeMethodSquashShort";
    case "rebase":
      return "github:mergeMethodRebaseShort";
    case "merge":
      return "github:mergeMethodMergeCommitShort";
  }
}

// Returns the methods the repo allows, in the order we prefer to surface
// them (squash → merge → rebase, matching GitHub's own button defaults).
// When the lookup hasn't resolved (or failed), returns an empty array so
// the button still renders without a dropdown and the backend's resolver
// picks the method at merge time.
function allowedMethods(
  methods: { merge: boolean; squash: boolean; rebase: boolean } | null,
): MergeMethod[] {
  if (!methods) return [];
  const order: MergeMethod[] = ["squash", "merge", "rebase"];
  return order.filter((m) => methods[m]);
}

function pickPrimaryMethod(allowed: MergeMethod[]): MergeMethod | undefined {
  return allowed[0];
}

"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconInfoCircle } from "@tabler/icons-react";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { BranchSelector } from "@/components/branch-selector";
import {
  branchToOption,
  buildBranchKeywords,
  sortBranches,
} from "@/components/branch-picker-options";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type { Branch, RepositoryBranchPolicy } from "@/lib/types/http";

export type PolicyDraft = Omit<
  RepositoryBranchPolicy,
  "id" | "repository_id" | "created_at" | "updated_at"
>;

export const DEFAULT_POLICY_DRAFT: PolicyDraft = {
  name: "",
  description: "",
  base_branch: "",
  // i18n-exempt: branch template protocol
  branch_template: "feature/{title}-{suffix}",
  pull_request_target: "",
};

export function draftFromPolicy(policy?: RepositoryBranchPolicy): PolicyDraft {
  return policy
    ? {
        name: policy.name,
        description: policy.description,
        base_branch: policy.base_branch,
        branch_template: policy.branch_template,
        pull_request_target: policy.pull_request_target,
      }
    : { ...DEFAULT_POLICY_DRAFT };
}

export function FieldHelp({ label, description }: { label: string; description: string }) {
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const button = (
    <button
      type="button"
      className="min-h-11 min-w-11 cursor-pointer text-muted-foreground hover:text-foreground"
      aria-label={label}
      aria-haspopup={usesTouchDrawer ? "dialog" : undefined}
      aria-expanded={usesTouchDrawer ? open : undefined}
    >
      <IconInfoCircle className="mx-auto h-4 w-4" />
    </button>
  );
  if (!usesTouchDrawer) {
    return (
      <TooltipProvider delayDuration={150}>
        <Tooltip>
          <TooltipTrigger asChild>{button}</TooltipTrigger>
          <TooltipContent className="w-80 text-xs">{description}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    );
  }
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <DrawerTrigger asChild>{button}</DrawerTrigger>
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle>{label}</DrawerTitle>
          <DrawerDescription>{description}</DrawerDescription>
        </DrawerHeader>
      </DrawerContent>
    </Drawer>
  );
}

function selectedFallbackOption(value: string) {
  return {
    value,
    label: value,
    keywords: buildBranchKeywords(value),
    renderLabel: () => <span className="truncate">{value}</span>,
  };
}

export function BranchPolicyBranchPicker({
  label,
  value,
  onChange,
  branches,
  onRefresh,
  loading,
  testId,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  branches: Branch[];
  onRefresh?: () => void;
  loading: boolean;
  testId: string;
}) {
  const { t } = useTranslation();
  const options = useMemo(() => {
    const available = sortBranches(branches).map(branchToOption);
    if (!value || available.some((option) => option.value === value)) return available;
    return [selectedFallbackOption(value), ...available];
  }, [branches, value]);

  return (
    <BranchSelector
      options={options}
      value={value}
      onValueChange={onChange}
      disabled={loading || options.length === 0}
      placeholder={loading ? t("common:loading") : t("task:noBranchesShort")}
      searchPlaceholder={t("task:searchBranches")}
      emptyMessage={t("task:noBranches")}
      onRefresh={onRefresh}
      refreshing={loading}
      loading={loading}
      ariaLabel={label}
      dropdownLabel={label}
      testId={testId}
      triggerClassName="min-h-11 border border-input bg-background px-3 hover:bg-background"
    />
  );
}

export function BranchPolicyBranchField({
  label,
  value,
  onChange,
  branches,
  onRefresh,
  loading,
  helpLabel,
  help,
  testId,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  branches: Branch[];
  onRefresh?: () => void;
  loading: boolean;
  helpLabel: string;
  help: string;
  testId: string;
}) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-1">
        <Label>{label}</Label>
        <FieldHelp label={helpLabel} description={help} />
      </div>
      <BranchPolicyBranchPicker
        label={label}
        value={value}
        onChange={onChange}
        branches={branches}
        onRefresh={onRefresh}
        loading={loading}
        testId={testId}
      />
    </div>
  );
}

export function PolicyFields({
  draft,
  setDraft,
  branches,
  refreshBranches,
  branchesLoading,
  idPrefix,
}: {
  draft: PolicyDraft;
  setDraft: (next: PolicyDraft) => void;
  branches: Branch[];
  refreshBranches?: () => void;
  branchesLoading: boolean;
  idPrefix: string;
}) {
  const { t } = useTranslation();
  const fieldId = (name: string) => `${idPrefix}-${name}`;
  const update = (key: keyof PolicyDraft, value: string) => setDraft({ ...draft, [key]: value });
  return (
    <div className="space-y-4">
      <div className="space-y-1.5">
        <div className="flex items-center gap-1">
          <Label htmlFor={fieldId("name")}>{t("workspaces:branchPolicyName")}</Label>
          <FieldHelp
            label={t("workspaces:branchPolicyNameHelpLabel")}
            description={t("workspaces:branchPolicyNameHelp")}
          />
        </div>
        <Input
          id={fieldId("name")}
          value={draft.name}
          onChange={(event) => update("name", event.target.value)}
          maxLength={100}
          autoFocus
        />
        <p className="text-xs text-muted-foreground">{t("workspaces:branchPolicyNameHint")}</p>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={fieldId("description")}>{t("workspaces:branchPolicyDescription")}</Label>
        <Input
          id={fieldId("description")}
          value={draft.description}
          onChange={(event) => update("description", event.target.value)}
          maxLength={500}
        />
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <BranchPolicyBranchField
          label={t("workspaces:branchPolicyBaseBranch")}
          value={draft.base_branch}
          onChange={(value) => update("base_branch", value)}
          branches={branches}
          onRefresh={refreshBranches}
          loading={branchesLoading}
          helpLabel={t("workspaces:branchPolicyBaseBranchHelpLabel")}
          help={t("workspaces:branchPolicyBaseBranchHelp")}
          testId={fieldId("base-picker")}
        />
        <BranchPolicyBranchField
          label={t("workspaces:branchPolicyPullRequestTarget")}
          value={draft.pull_request_target}
          onChange={(value) => update("pull_request_target", value)}
          branches={branches}
          onRefresh={refreshBranches}
          loading={branchesLoading}
          helpLabel={t("workspaces:branchPolicyPullRequestTargetHelpLabel")}
          help={t("workspaces:branchPolicyPullRequestTargetHelp")}
          testId={fieldId("target-picker")}
        />
      </div>
      <div className="space-y-1.5">
        <div className="flex items-center gap-1">
          <Label htmlFor={fieldId("template")}>{t("workspaces:branchPolicyTemplate")}</Label>
          <FieldHelp
            label={t("workspaces:branchPolicyTemplateHelpLabel")}
            description={t("workspaces:branchPolicyTemplateHelp")}
          />
        </div>
        <Input
          id={fieldId("template")}
          value={draft.branch_template}
          onChange={(event) => update("branch_template", event.target.value)}
        />
        <p className="text-xs text-muted-foreground">{t("workspaces:branchPolicyTemplateHint")}</p>
      </div>
    </div>
  );
}

"use client";

/* eslint-disable max-lines -- Co-locates the policy form, Gitflow starter, and CRUD surface. */

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  IconChevronDown,
  IconInfoCircle,
  IconPencil,
  IconPlus,
  IconTrash,
} from "@tabler/icons-react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { HoverCard, HoverCardContent, HoverCardTrigger } from "@kandev/ui/hover-card";
import { useToast } from "@/components/toast-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useBranches } from "@/hooks/domains/workspace/use-repository-branches";
import { useRepositoryBranchPolicies } from "@/hooks/domains/workspace/use-repository-branch-policies";
import type { Repository, RepositoryBranchPolicy } from "@/lib/types/http";

type PolicyDraft = Omit<
  RepositoryBranchPolicy,
  "id" | "repository_id" | "created_at" | "updated_at"
>;

const TOUCH_TARGET_CLASS = "min-h-11";
const CANCEL_LABEL_KEY = "common:cancel";

const DEFAULT_POLICY_DRAFT: PolicyDraft = {
  name: "",
  description: "",
  base_branch: "",
  // i18n-exempt: branch template protocol
  branch_template: "feature/{title}-{suffix}",
  pull_request_target: "",
};

function draftFromPolicy(policy?: RepositoryBranchPolicy): PolicyDraft {
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

function FieldHelp({ label, description }: { label: string; description: string }) {
  return (
    <HoverCard openDelay={150} closeDelay={100}>
      <HoverCardTrigger asChild>
        <button
          type="button"
          className={`${TOUCH_TARGET_CLASS} min-w-11 text-muted-foreground hover:text-foreground`}
          aria-label={label}
        >
          <IconInfoCircle className="mx-auto h-4 w-4" />
        </button>
      </HoverCardTrigger>
      <HoverCardContent align="start" className="w-80 text-xs">
        {description}
      </HoverCardContent>
    </HoverCard>
  );
}

function BranchField({
  id,
  label,
  value,
  onChange,
  options,
  helpLabel,
  help,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: string[];
  helpLabel: string;
  help: string;
}) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-1">
        <Label htmlFor={id}>{label}</Label>
        <FieldHelp label={helpLabel} description={help} />
      </div>
      <Input
        id={id}
        list={`${id}-options`}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
      <datalist id={`${id}-options`}>
        {options.map((option) => (
          <option key={option} value={option} />
        ))}
      </datalist>
    </div>
  );
}

function PolicyFields({
  draft,
  setDraft,
  branchOptions,
}: {
  draft: PolicyDraft;
  setDraft: (next: PolicyDraft) => void;
  branchOptions: string[];
}) {
  const { t } = useTranslation();
  const update = (key: keyof PolicyDraft, value: string) => setDraft({ ...draft, [key]: value });
  return (
    <div className="space-y-4">
      <div className="space-y-1.5">
        <div className="flex items-center gap-1">
          <Label htmlFor="branch-policy-name">{t("workspaces:branchPolicyName")}</Label>
          <FieldHelp
            label={t("workspaces:branchPolicyNameHelpLabel")}
            description={t("workspaces:branchPolicyNameHelp")}
          />
        </div>
        <Input
          id="branch-policy-name"
          value={draft.name}
          onChange={(event) => update("name", event.target.value)}
          maxLength={100}
          autoFocus
        />
        <p className="text-xs text-muted-foreground">{t("workspaces:branchPolicyNameHint")}</p>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="branch-policy-description">{t("workspaces:branchPolicyDescription")}</Label>
        <Input
          id="branch-policy-description"
          value={draft.description}
          onChange={(event) => update("description", event.target.value)}
          maxLength={500}
        />
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <BranchField
          id="branch-policy-base"
          label={t("workspaces:branchPolicyBaseBranch")}
          value={draft.base_branch}
          onChange={(value) => update("base_branch", value)}
          options={branchOptions}
          helpLabel={t("workspaces:branchPolicyBaseBranchHelpLabel")}
          help={t("workspaces:branchPolicyBaseBranchHelp")}
        />
        <BranchField
          id="branch-policy-target"
          label={t("workspaces:branchPolicyPullRequestTarget")}
          value={draft.pull_request_target}
          onChange={(value) => update("pull_request_target", value)}
          options={branchOptions}
          helpLabel={t("workspaces:branchPolicyPullRequestTargetHelpLabel")}
          help={t("workspaces:branchPolicyPullRequestTargetHelp")}
        />
      </div>
      <div className="space-y-1.5">
        <div className="flex items-center gap-1">
          <Label htmlFor="branch-policy-template">{t("workspaces:branchPolicyTemplate")}</Label>
          <FieldHelp
            label={t("workspaces:branchPolicyTemplateHelpLabel")}
            description={t("workspaces:branchPolicyTemplateHelp")}
          />
        </div>
        <Input
          id="branch-policy-template"
          value={draft.branch_template}
          onChange={(event) => update("branch_template", event.target.value)}
        />
        <p className="text-xs text-muted-foreground">{t("workspaces:branchPolicyTemplateHint")}</p>
      </div>
    </div>
  );
}

function PolicySurface({
  open,
  onOpenChange,
  title,
  description,
  draft,
  setDraft,
  branchOptions,
  onSubmit,
  loading,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  draft: PolicyDraft;
  setDraft: (next: PolicyDraft) => void;
  branchOptions: string[];
  onSubmit: () => void;
  loading: boolean;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const body = (
    <form
      id="branch-policy-form"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit();
      }}
      className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-4"
    >
      <PolicyFields draft={draft} setDraft={setDraft} branchOptions={branchOptions} />
    </form>
  );
  const footer = isMobile ? (
    <DrawerFooter className="shrink-0 border-t px-4 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))]">
      <Button
        type="submit"
        form="branch-policy-form"
        disabled={loading}
        className={TOUCH_TARGET_CLASS}
      >
        {t("common:save")}
      </Button>
      <Button
        type="button"
        variant="outline"
        onClick={() => onOpenChange(false)}
        className={TOUCH_TARGET_CLASS}
      >
        {t(CANCEL_LABEL_KEY)}
      </Button>
    </DrawerFooter>
  ) : (
    <DialogFooter>
      <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
        {t(CANCEL_LABEL_KEY)}
      </Button>
      <Button type="submit" form="branch-policy-form" disabled={loading}>
        {t("common:save")}
      </Button>
    </DialogFooter>
  );
  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent className="flex h-[100dvh] max-h-[100dvh] flex-col overflow-hidden">
          <DrawerHeader className="shrink-0 px-4 py-3 text-left">
            <DrawerTitle>{title}</DrawerTitle>
            <DrawerDescription>{description}</DrawerDescription>
          </DrawerHeader>
          {body}
          {footer}
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[92dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-2xl">
        <DialogHeader className="shrink-0 px-4 pb-1 pt-3 text-left">
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {body}
        {footer}
      </DialogContent>
    </Dialog>
  );
}

// eslint-disable-next-line max-lines-per-function -- Shares validated fields and one responsive surface.
function GitflowSurface({
  open,
  onOpenChange,
  productionBranch,
  developmentBranch,
  setProductionBranch,
  setDevelopmentBranch,
  branchOptions,
  branchesLoading,
  onSubmit,
  loading,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  productionBranch: string;
  developmentBranch: string;
  setProductionBranch: (value: string) => void;
  setDevelopmentBranch: (value: string) => void;
  branchOptions: string[];
  branchesLoading: boolean;
  onSubmit: () => void;
  loading: boolean;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const fields = (
    <div className="space-y-4 px-4 py-4">
      <p className="text-sm text-muted-foreground">
        {t("workspaces:branchPolicyGitflowDescription")}
      </p>
      <BranchField
        id="gitflow-production"
        label={t("workspaces:branchPolicyProductionBranch")}
        value={productionBranch}
        onChange={setProductionBranch}
        options={branchOptions}
        helpLabel={t("workspaces:branchPolicyProductionHelpLabel")}
        help={t("workspaces:branchPolicyProductionHelp")}
      />
      <BranchField
        id="gitflow-development"
        label={t("workspaces:branchPolicyDevelopmentBranch")}
        value={developmentBranch}
        onChange={setDevelopmentBranch}
        options={branchOptions}
        helpLabel={t("workspaces:branchPolicyDevelopmentHelpLabel")}
        help={t("workspaces:branchPolicyDevelopmentHelp")}
      />
      <p className="text-xs text-muted-foreground">{t("workspaces:branchPolicyGitflowPreview")}</p>
    </div>
  );
  const valid =
    !branchesLoading &&
    productionBranch.trim() !== "" &&
    developmentBranch.trim() !== "" &&
    productionBranch.trim() !== developmentBranch.trim() &&
    branchOptions.includes(productionBranch.trim()) &&
    branchOptions.includes(developmentBranch.trim());
  const footer = isMobile ? (
    <DrawerFooter className="border-t px-4 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))]">
      <Button
        type="button"
        onClick={onSubmit}
        disabled={!valid || loading}
        className={TOUCH_TARGET_CLASS}
      >
        {t("workspaces:branchPolicyGitflowCreate")}
      </Button>
      <Button
        type="button"
        variant="outline"
        onClick={() => onOpenChange(false)}
        className={TOUCH_TARGET_CLASS}
      >
        {t(CANCEL_LABEL_KEY)}
      </Button>
    </DrawerFooter>
  ) : (
    <DialogFooter>
      <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
        {t(CANCEL_LABEL_KEY)}
      </Button>
      <Button type="button" onClick={onSubmit} disabled={!valid || loading}>
        {t("workspaces:branchPolicyGitflowCreate")}
      </Button>
    </DialogFooter>
  );
  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent className="flex h-[100dvh] max-h-[100dvh] flex-col overflow-hidden">
          <DrawerHeader className="shrink-0 px-4 py-3 text-left">
            <DrawerTitle>{t("workspaces:branchPolicyGitflowTitle")}</DrawerTitle>
            <DrawerDescription>{t("workspaces:branchPolicyGitflowDescription")}</DrawerDescription>
          </DrawerHeader>
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain">{fields}</div>
          {footer}
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("workspaces:branchPolicyGitflowTitle")}</DialogTitle>
          <DialogDescription>{t("workspaces:branchPolicyGitflowDescription")}</DialogDescription>
        </DialogHeader>
        {fields}
        {footer}
      </DialogContent>
    </Dialog>
  );
}

// eslint-disable-next-line max-lines-per-function -- Coordinates policy CRUD, Gitflow seeding, and editors.
export function RepositoryBranchPolicies({
  repository,
  workspaceId,
}: {
  repository: Repository;
  workspaceId: string;
}) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const isSaved = !repository.id.startsWith("temp-repo-");
  const { policies, isLoading, create, update, remove, seedGitflow } = useRepositoryBranchPolicies(
    repository.id,
    isSaved,
  );
  const { branches, isLoading: branchesLoading } = useBranches(
    isSaved ? { kind: "id", workspaceId, repositoryId: repository.id } : null,
    isSaved,
  );
  const branchOptions = useMemo(() => {
    const names = branches.map((branch) => branch.name).filter(Boolean);
    return [...new Set(names)].sort((left, right) => left.localeCompare(right));
  }, [branches]);
  const [editorPolicy, setEditorPolicy] = useState<RepositoryBranchPolicy | undefined>();
  const [draft, setDraft] = useState<PolicyDraft>(() => draftFromPolicy());
  const [editorOpen, setEditorOpen] = useState(false);
  const [gitflowOpen, setGitflowOpen] = useState(false);
  const [productionBranch, setProductionBranch] = useState("");
  const [developmentBranch, setDevelopmentBranch] = useState("");
  const [pending, setPending] = useState(false);
  const [deletePolicy, setDeletePolicy] = useState<RepositoryBranchPolicy | undefined>();

  const openCreate = () => {
    setEditorPolicy(undefined);
    setDraft(draftFromPolicy());
    setEditorOpen(true);
  };
  const openEdit = (policy: RepositoryBranchPolicy) => {
    setEditorPolicy(policy);
    setDraft(draftFromPolicy(policy));
    setEditorOpen(true);
  };
  const submitPolicy = async () => {
    setPending(true);
    try {
      if (editorPolicy) await update(editorPolicy.id, draft);
      else await create(draft);
      setEditorOpen(false);
      toast({ title: t("workspaces:branchPolicySaved"), variant: "success" });
    } catch (error) {
      toast({
        title: t("workspaces:branchPolicySaveFailed"),
        description: error instanceof Error ? error.message : t("common:requestFailed"),
        variant: "error",
      });
    } finally {
      setPending(false);
    }
  };
  const submitGitflow = async () => {
    setPending(true);
    try {
      await seedGitflow(productionBranch.trim(), developmentBranch.trim());
      setGitflowOpen(false);
      toast({ title: t("workspaces:branchPolicyGitflowCreated"), variant: "success" });
    } catch (error) {
      toast({
        title: t("workspaces:branchPolicyGitflowFailed"),
        description: error instanceof Error ? error.message : t("common:requestFailed"),
        variant: "error",
      });
    } finally {
      setPending(false);
    }
  };
  const confirmDelete = async () => {
    if (!deletePolicy) return;
    setPending(true);
    try {
      await remove(deletePolicy.id);
      setDeletePolicy(undefined);
      toast({ title: t("workspaces:branchPolicyDeleted"), variant: "success" });
    } catch (error) {
      toast({
        title: t("workspaces:branchPolicyDeleteFailed"),
        description: error instanceof Error ? error.message : t("common:requestFailed"),
        variant: "error",
      });
    } finally {
      setPending(false);
    }
  };

  return (
    <details
      className="rounded-md border border-border/70 p-3"
      data-testid={`branch-policies-${repository.id}`}
    >
      <summary className="flex min-h-11 cursor-pointer list-none items-center justify-between gap-3 [&::-webkit-details-marker]:hidden">
        <span className="flex items-center gap-2 font-medium">
          <IconChevronDown className="h-4 w-4 transition-transform details-open:rotate-180" />
          {t("workspaces:branchPoliciesTitle")}
          <span className="rounded-full bg-muted px-2 py-0.5 text-xs tabular-nums">
            {policies.length}
          </span>
        </span>
      </summary>
      <div className="space-y-3 pt-3">
        <p className="text-sm text-muted-foreground">{t("workspaces:branchPoliciesDescription")}</p>
        {!isSaved ? (
          <p className="text-xs text-muted-foreground">
            {t("workspaces:branchPoliciesSaveRepositoryFirst")}
          </p>
        ) : null}
        {isSaved && policies.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("workspaces:branchPoliciesEmpty")}</p>
        ) : null}
        <div className="space-y-2">
          {policies.map((policy) => (
            <div
              key={policy.id}
              className="flex flex-wrap items-center justify-between gap-2 rounded-md bg-muted/40 p-3"
              data-testid={`branch-policy-${policy.id}`}
            >
              <div className="min-w-0 space-y-0.5">
                <p className="font-medium">{policy.name}</p>
                <p className="text-xs text-muted-foreground">
                  {t("workspaces:branchPolicySummary", {
                    base: policy.base_branch,
                    template: policy.branch_template,
                    target: policy.pull_request_target,
                  })}
                </p>
                {policy.description ? (
                  <p className="text-xs text-muted-foreground">{policy.description}</p>
                ) : null}
              </div>
              <div className="flex items-center gap-1">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-11 w-11"
                  onClick={() => openEdit(policy)}
                  aria-label={t("workspaces:branchPolicyEdit", { name: policy.name })}
                >
                  <IconPencil className="h-4 w-4" />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-11 w-11"
                  onClick={() => setDeletePolicy(policy)}
                  aria-label={t("workspaces:branchPolicyDelete", { name: policy.name })}
                >
                  <IconTrash className="h-4 w-4" />
                </Button>
              </div>
            </div>
          ))}
        </div>
        {isSaved ? (
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              variant="outline"
              className={TOUCH_TARGET_CLASS}
              onClick={openCreate}
            >
              <IconPlus className="mr-2 h-4 w-4" />
              {t("workspaces:branchPolicyAdd")}
            </Button>
            {policies.length === 0 ? (
              <Button
                type="button"
                variant="outline"
                className={TOUCH_TARGET_CLASS}
                onClick={() => {
                  setProductionBranch(repository.default_branch || branchOptions[0] || "");
                  setDevelopmentBranch(
                    branchOptions.includes("develop")
                      ? "develop"
                      : branchOptions.find(
                          (branch) => branch !== (repository.default_branch || branchOptions[0]),
                        ) || "",
                  );
                  setGitflowOpen(true);
                }}
              >
                {t("workspaces:branchPolicyGitflowAdd")}
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
      <PolicySurface
        open={editorOpen}
        onOpenChange={setEditorOpen}
        title={
          editorPolicy
            ? t("workspaces:branchPolicyEditTitle")
            : t("workspaces:branchPolicyAddTitle")
        }
        description={t("workspaces:branchPolicyEditorDescription")}
        draft={draft}
        setDraft={setDraft}
        branchOptions={branchOptions}
        onSubmit={() => void submitPolicy()}
        loading={pending}
      />
      <GitflowSurface
        open={gitflowOpen}
        onOpenChange={setGitflowOpen}
        productionBranch={productionBranch}
        developmentBranch={developmentBranch}
        setProductionBranch={setProductionBranch}
        setDevelopmentBranch={setDevelopmentBranch}
        branchOptions={branchOptions}
        branchesLoading={branchesLoading}
        onSubmit={() => void submitGitflow()}
        loading={pending}
      />
      <AlertDialog
        open={Boolean(deletePolicy)}
        onOpenChange={(open) => {
          if (!open) setDeletePolicy(undefined);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("workspaces:branchPolicyDeleteTitle", { name: deletePolicy?.name ?? "" })}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("workspaces:branchPolicyDeleteDescription")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="cursor-pointer">{t(CANCEL_LABEL_KEY)}</AlertDialogCancel>
            <AlertDialogAction
              className="cursor-pointer"
              disabled={pending}
              onClick={() => void confirmDelete()}
            >
              {t("workspaces:branchPolicyDeleteConfirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      {isLoading ? (
        <p className="pt-2 text-xs text-muted-foreground">
          {t("workspaces:branchPoliciesLoading")}
        </p>
      ) : null}
    </details>
  );
}

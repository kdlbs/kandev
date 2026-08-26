"use client";

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconPencil, IconPlus, IconTrash } from "@tabler/icons-react";
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
import { useToast } from "@/components/toast-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useBranches } from "@/hooks/domains/workspace/use-repository-branches";
import { useRepositoryBranchPolicies } from "@/hooks/domains/workspace/use-repository-branch-policies";
import type { Repository, RepositoryBranchPolicy } from "@/lib/types/http";
import { branchToOption, sortBranches } from "@/components/task-create-dialog-branch-options";
import {
  BranchPolicyBranchField,
  PolicyFields,
  draftFromPolicy,
  type PolicyDraft,
} from "@/components/settings/repository-branch-policy-fields";

const TOUCH_TARGET_CLASS = "min-h-11";
const CANCEL_LABEL_KEY = "common:cancel";

function PolicySurface({
  open,
  onOpenChange,
  title,
  description,
  draft,
  setDraft,
  branches,
  refreshBranches,
  branchesLoading,
  idPrefix,
  onSubmit,
  loading,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  draft: PolicyDraft;
  setDraft: (next: PolicyDraft) => void;
  branches: ReturnType<typeof useBranches>["branches"];
  refreshBranches?: () => void;
  branchesLoading: boolean;
  idPrefix: string;
  onSubmit: () => void;
  loading: boolean;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const formId = `${idPrefix}-form`;
  const body = (
    <form
      id={formId}
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit();
      }}
      className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-4"
    >
      <PolicyFields
        draft={draft}
        setDraft={setDraft}
        branches={branches}
        refreshBranches={refreshBranches}
        branchesLoading={branchesLoading}
        idPrefix={idPrefix}
      />
    </form>
  );
  const footer = isMobile ? (
    <DrawerFooter className="shrink-0 border-t px-4 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))]">
      <Button type="submit" form={formId} disabled={loading} className={TOUCH_TARGET_CLASS}>
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
      <Button type="submit" form={formId} disabled={loading}>
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
  branches,
  branchOptions,
  refreshBranches,
  idPrefix,
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
  branches: ReturnType<typeof useBranches>["branches"];
  refreshBranches?: () => void;
  idPrefix: string;
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
      <BranchPolicyBranchField
        label={t("workspaces:branchPolicyProductionBranch")}
        value={productionBranch}
        onChange={setProductionBranch}
        branches={branches}
        onRefresh={refreshBranches}
        loading={branchesLoading}
        helpLabel={t("workspaces:branchPolicyProductionHelpLabel")}
        help={t("workspaces:branchPolicyProductionHelp")}
        testId={`${idPrefix}-gitflow-production-picker`}
      />
      <BranchPolicyBranchField
        label={t("workspaces:branchPolicyDevelopmentBranch")}
        value={developmentBranch}
        onChange={setDevelopmentBranch}
        branches={branches}
        onRefresh={refreshBranches}
        loading={branchesLoading}
        helpLabel={t("workspaces:branchPolicyDevelopmentHelpLabel")}
        help={t("workspaces:branchPolicyDevelopmentHelp")}
        testId={`${idPrefix}-gitflow-development-picker`}
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
  const idPrefix = `branch-policy-${repository.id.replace(/[^a-zA-Z0-9_-]/g, "-")}`;
  const { policies, hasError, isLoading, refresh, create, update, remove, seedGitflow } =
    useRepositoryBranchPolicies(repository.id, isSaved);
  const {
    branches,
    isLoading: branchesLoading,
    refresh: refreshBranches,
  } = useBranches(
    isSaved ? { kind: "id", workspaceId, repositoryId: repository.id } : null,
    isSaved,
  );
  const branchOptions = useMemo(() => {
    const names = sortBranches(branches).map((branch) => branchToOption(branch).value);
    return [...new Set(names)];
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
      className="group rounded-md border border-border/70 p-3"
      data-testid={`branch-policies-${repository.id}`}
    >
      <summary className="flex min-h-11 cursor-pointer list-none items-center justify-between gap-3 [&::-webkit-details-marker]:hidden">
        <span className="flex items-center gap-2 font-medium">
          <IconChevronDown className="h-4 w-4 transition-transform group-open:rotate-180" />
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
        {isSaved && hasError ? (
          <div className="flex flex-wrap items-center gap-2">
            <p className="text-sm text-destructive">{t("workspaces:branchPoliciesLoadFailed")}</p>
            <Button
              type="button"
              variant="outline"
              className={TOUCH_TARGET_CLASS}
              onClick={() => void refresh().catch(() => undefined)}
            >
              {t("workspaces:branchPoliciesRetry")}
            </Button>
          </div>
        ) : null}
        {isSaved && !hasError && policies.length === 0 ? (
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
            {!hasError && policies.length === 0 ? (
              <Button
                type="button"
                variant="outline"
                className={TOUCH_TARGET_CLASS}
                disabled={branchesLoading}
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
        branches={branches}
        refreshBranches={refreshBranches}
        branchesLoading={branchesLoading}
        idPrefix={idPrefix}
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
        branches={branches}
        branchOptions={branchOptions}
        refreshBranches={refreshBranches}
        idPrefix={idPrefix}
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

"use client";

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
import { Textarea } from "@kandev/ui/textarea";
import { useTranslation } from "react-i18next";

import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { Repository } from "@/lib/types/http";
import { RepositorySetMembersField } from "./workspace-repository-set-editor-members";
import type { RepositorySetDraft } from "./use-workspace-repository-sets";

type RepositorySetEditorDialogProps = {
  workspaceId: string;
  draft: RepositorySetDraft | null;
  repositories: Repository[];
  error: string | null;
  saving: boolean;
  onClose: () => void;
  onChange: (patch: Partial<RepositorySetDraft>) => void;
  onSubmit: () => void;
};

type RepositorySetEditorFooterProps = {
  formId: string;
  canSave: boolean;
  isMobile: boolean;
  onClose: () => void;
};

function RepositorySetEditorFooter({
  formId,
  canSave,
  isMobile,
  onClose,
}: RepositorySetEditorFooterProps) {
  const { t } = useTranslation();
  if (isMobile) {
    return (
      <DrawerFooter className="shrink-0 border-t px-4 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))]">
        <Button
          type="submit"
          form={formId}
          disabled={!canSave}
          className="min-h-11 cursor-pointer"
          data-testid="repository-set-editor-save"
        >
          {t("workspaces:repositorySetsSave")}
        </Button>
        <Button
          type="button"
          variant="outline"
          onClick={onClose}
          className="min-h-11 cursor-pointer"
          data-testid="repository-set-editor-cancel"
        >
          {t("common:cancel")}
        </Button>
      </DrawerFooter>
    );
  }
  return (
    <DialogFooter>
      <Button
        type="button"
        variant="outline"
        onClick={onClose}
        className="cursor-pointer"
        data-testid="repository-set-editor-cancel"
      >
        {t("common:cancel")}
      </Button>
      <Button
        type="submit"
        form={formId}
        disabled={!canSave}
        className="cursor-pointer"
        data-testid="repository-set-editor-save"
      >
        {t("workspaces:repositorySetsSave")}
      </Button>
    </DialogFooter>
  );
}

/**
 * Creates or edits one set. The desktop dialog and phone drawer share the same
 * form body, so member order and saved-base behavior do not diverge by device.
 */
export function RepositorySetEditorDialog({
  workspaceId,
  draft,
  repositories,
  error,
  saving,
  onClose,
  onChange,
  onSubmit,
}: RepositorySetEditorDialogProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  if (!draft) return null;

  const formId = "repository-set-editor-form";
  const canSave = draft.name.trim().length > 0 && draft.members.length > 0 && !saving;
  const body = (
    <RepositorySetEditorBody
      workspaceId={workspaceId}
      draft={draft}
      repositories={repositories}
      error={error}
      formId={formId}
      onChange={onChange}
      onSubmit={onSubmit}
    />
  );
  const footer = (
    <RepositorySetEditorFooter
      formId={formId}
      canSave={canSave}
      isMobile={isMobile}
      onClose={onClose}
    />
  );

  if (isMobile) {
    return (
      <Drawer open onOpenChange={(open) => !open && onClose()}>
        <DrawerContent
          className="flex h-[100dvh] max-h-[100dvh] flex-col overflow-hidden"
          data-testid="repository-set-editor-surface"
        >
          <DrawerHeader className="shrink-0 px-4 py-3 text-left">
            <DrawerTitle>
              {draft.setId
                ? t("workspaces:repositorySetsEditTitle")
                : t("workspaces:repositorySetsCreateTitle")}
            </DrawerTitle>
            <DrawerDescription>{t("workspaces:repositorySetsEditorDescription")}</DrawerDescription>
          </DrawerHeader>
          {body}
          {footer}
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent
        className="flex max-h-[92dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-2xl"
        data-testid="repository-set-editor-surface"
      >
        <DialogHeader className="shrink-0 px-4 pb-1 pt-3 text-left">
          <DialogTitle>
            {draft.setId
              ? t("workspaces:repositorySetsEditTitle")
              : t("workspaces:repositorySetsCreateTitle")}
          </DialogTitle>
          <DialogDescription>{t("workspaces:repositorySetsEditorDescription")}</DialogDescription>
        </DialogHeader>
        {body}
        {footer}
      </DialogContent>
    </Dialog>
  );
}

type RepositorySetEditorBodyProps = {
  workspaceId: string;
  draft: RepositorySetDraft;
  repositories: Repository[];
  error: string | null;
  formId: string;
  onChange: (patch: Partial<RepositorySetDraft>) => void;
  onSubmit: () => void;
};

function RepositorySetEditorBody({
  workspaceId,
  draft,
  repositories,
  error,
  formId,
  onChange,
  onSubmit,
}: RepositorySetEditorBodyProps) {
  const { t } = useTranslation();

  return (
    <form
      id={formId}
      data-testid="repository-set-editor-form"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit();
      }}
      className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-4"
    >
      <div className="space-y-4">
        <label
          className="block space-y-1.5 text-xs font-medium"
          htmlFor="repository-set-editor-name"
        >
          <span>{t("workspaces:repositorySetsNameLabel")}</span>
          <Input
            id="repository-set-editor-name"
            value={draft.name}
            maxLength={100}
            autoFocus
            onChange={(event) => onChange({ name: event.target.value })}
            data-testid="repository-set-editor-name"
          />
        </label>
        <label
          className="block space-y-1.5 text-xs font-medium"
          htmlFor="repository-set-editor-description"
        >
          <span>{t("workspaces:repositorySetsDescriptionLabel")}</span>
          <Textarea
            id="repository-set-editor-description"
            value={draft.description}
            rows={2}
            onChange={(event) => onChange({ description: event.target.value })}
            data-testid="repository-set-editor-description"
          />
        </label>
        <RepositorySetMembersField
          workspaceId={workspaceId}
          draft={draft}
          repositories={repositories}
          onChange={onChange}
        />
        {error ? (
          <p className="text-xs text-destructive" data-testid="repository-set-editor-error">
            {error}
          </p>
        ) : null}
      </div>
    </form>
  );
}

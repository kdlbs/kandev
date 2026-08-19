"use client";

import { IconArrowDown, IconArrowUp } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { useTranslation } from "react-i18next";

import type { Repository } from "@/lib/types/http";
import type { RepositorySetDraft } from "./use-workspace-repository-sets";

type RepositorySetEditorDialogProps = {
  draft: RepositorySetDraft | null;
  repositories: Repository[];
  error: string | null;
  saving: boolean;
  onClose: () => void;
  onChange: (patch: Partial<RepositorySetDraft>) => void;
  onSubmit: () => void;
};

/**
 * Creates or edits one set: name, description, and which repositories belong in
 * which order. Member selection offers only this workspace's live repositories,
 * so a set can never be given a member the backend would reject.
 */
export function RepositorySetEditorDialog({
  draft,
  repositories,
  error,
  saving,
  onClose,
  onChange,
  onSubmit,
}: RepositorySetEditorDialogProps) {
  const { t } = useTranslation();
  if (!draft) return null;
  const canSave = draft.name.trim().length > 0 && draft.repositoryIds.length > 0 && !saving;

  return (
    <Dialog open onOpenChange={(next) => (next ? undefined : onClose())}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {draft.setId
              ? t("workspaces:repositorySetsEditTitle")
              : t("workspaces:repositorySetsCreateTitle")}
          </DialogTitle>
          <DialogDescription>{t("workspaces:repositorySetsEditorDescription")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <label className="block space-y-1.5 text-xs font-medium">
            <span>{t("workspaces:repositorySetsNameLabel")}</span>
            <Input
              value={draft.name}
              maxLength={100}
              autoFocus
              onChange={(event) => onChange({ name: event.target.value })}
              data-testid="repository-set-editor-name"
            />
          </label>
          <label className="block space-y-1.5 text-xs font-medium">
            <span>{t("workspaces:repositorySetsDescriptionLabel")}</span>
            <Textarea
              value={draft.description}
              rows={2}
              onChange={(event) => onChange({ description: event.target.value })}
              data-testid="repository-set-editor-description"
            />
          </label>
          <MemberPicker
            repositories={repositories}
            selectedIds={draft.repositoryIds}
            onChange={(repositoryIds) => onChange({ repositoryIds })}
          />
          {error ? (
            <p className="text-xs text-destructive" data-testid="repository-set-editor-error">
              {error}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button
            type="button"
            className="cursor-pointer"
            disabled={!canSave}
            onClick={onSubmit}
            data-testid="repository-set-editor-save"
          >
            {t("workspaces:repositorySetsSave")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type MemberPickerProps = {
  repositories: Repository[];
  selectedIds: string[];
  onChange: (repositoryIds: string[]) => void;
};

/**
 * Which repositories are in the set, and in what order. Selected members are
 * listed first in their own order so reordering is visible; the rest follow.
 */
function MemberPicker({ repositories, selectedIds, onChange }: MemberPickerProps) {
  const { t } = useTranslation();
  const byId = new Map(repositories.map((repository) => [repository.id as string, repository]));
  const selected = selectedIds.filter((id) => byId.has(id));
  const unselected = repositories
    .map((repository) => repository.id as string)
    .filter((id) => !selected.includes(id));

  const toggle = (id: string) => {
    onChange(selected.includes(id) ? selected.filter((entry) => entry !== id) : [...selected, id]);
  };
  const move = (id: string, delta: number) => {
    const index = selected.indexOf(id);
    const target = index + delta;
    if (index === -1 || target < 0 || target >= selected.length) return;
    const next = [...selected];
    [next[index], next[target]] = [next[target], next[index]];
    onChange(next);
  };

  return (
    <div className="space-y-1.5">
      <p className="text-xs font-medium">{t("workspaces:repositorySetsMembersLabel")}</p>
      <p className="text-xs text-muted-foreground">{t("workspaces:repositorySetsMembersHint")}</p>
      <div className="max-h-64 space-y-1 overflow-y-auto rounded-md border p-2">
        {repositories.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            {t("workspaces:repositorySetsNoRepositories")}
          </p>
        ) : (
          [...selected, ...unselected].map((id) => (
            <MemberRow
              key={id}
              label={byId.get(id)?.name ?? id}
              repositoryId={id}
              checked={selected.includes(id)}
              canMoveUp={selected.indexOf(id) > 0}
              canMoveDown={
                selected.indexOf(id) !== -1 && selected.indexOf(id) < selected.length - 1
              }
              onToggle={() => toggle(id)}
              onMoveUp={() => move(id, -1)}
              onMoveDown={() => move(id, 1)}
            />
          ))
        )}
      </div>
    </div>
  );
}

type MemberRowProps = {
  label: string;
  repositoryId: string;
  checked: boolean;
  canMoveUp: boolean;
  canMoveDown: boolean;
  onToggle: () => void;
  onMoveUp: () => void;
  onMoveDown: () => void;
};

function MemberRow({
  label,
  repositoryId,
  checked,
  canMoveUp,
  canMoveDown,
  onToggle,
  onMoveUp,
  onMoveDown,
}: MemberRowProps) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-2 rounded px-1 py-1">
      {/* A 44px-tall label keeps the touch target usable on a phone. */}
      <label className="flex min-h-11 min-w-0 flex-1 cursor-pointer items-center gap-2 text-sm">
        <Checkbox
          checked={checked}
          onCheckedChange={onToggle}
          data-testid={`repository-set-member-${repositoryId}`}
        />
        <span className="truncate">{label}</span>
      </label>
      {checked ? (
        <div className="flex shrink-0 gap-0.5">
          <Button
            size="sm"
            variant="ghost"
            className="cursor-pointer"
            disabled={!canMoveUp}
            aria-label={t("workspaces:repositorySetsMoveUp")}
            onClick={onMoveUp}
            data-testid={`repository-set-move-up-${repositoryId}`}
          >
            <IconArrowUp className="h-4 w-4" />
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="cursor-pointer"
            disabled={!canMoveDown}
            aria-label={t("workspaces:repositorySetsMoveDown")}
            onClick={onMoveDown}
            data-testid={`repository-set-move-down-${repositoryId}`}
          >
            <IconArrowDown className="h-4 w-4" />
          </Button>
        </div>
      ) : null}
    </div>
  );
}

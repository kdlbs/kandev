"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconCopy, IconEdit, IconEye, IconEyeOff, IconKey, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { revealSecret } from "@/lib/api/domains/secrets-api";
import type { SecretListItem } from "@/lib/types/http-secrets";

type SecretListItemRowProps = {
  secret: SecretListItem;
  workspaceId?: string;
  onEdit: (secret: SecretListItem) => void;
  onDelete: (secret: SecretListItem) => void;
  onCopyMove: (secret: SecretListItem) => void;
  isBusy: boolean;
  showCreate: boolean;
  isEditing: boolean;
};

/** Renders a single secret row: name, scope badge, and copy/move, reveal, edit, and delete actions. */
export function SecretListItemRow({
  secret,
  workspaceId,
  onEdit,
  onDelete,
  onCopyMove,
  isBusy,
  showCreate,
  isEditing,
}: SecretListItemRowProps) {
  const { t } = useTranslation();
  const [revealed, setRevealed] = useState(false);
  const [revealedValue, setRevealedValue] = useState<string | null>(null);
  const [revealing, setRevealing] = useState(false);

  /** Fetches and shows the secret value, or hides it when already revealed. */
  const handleReveal = async () => {
    if (revealed) {
      setRevealed(false);
      setRevealedValue(null);
      return;
    }
    setRevealing(true);
    try {
      const resp = await revealSecret(secret.id, {
        cache: "no-store",
        ...(workspaceId ? { workspaceId } : {}),
      });
      setRevealedValue(resp.value);
      setRevealed(true);
    } catch {
      // ignore
    } finally {
      setRevealing(false);
    }
  };

  return (
    <div className="rounded-lg border border-border/70 bg-background p-4 space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2 min-w-0">
          <IconKey className="h-4 w-4 text-muted-foreground shrink-0" />
          <div className="text-sm font-medium text-foreground truncate">{secret.name}</div>
          <Badge variant="outline" className="shrink-0 text-[10px]">
            {secret.scope === "workspace"
              ? t("settings:workspaceScope")
              : t("settings:globalScope")}
          </Badge>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-1 shrink-0">
          <Button
            variant="ghost"
            onClick={() => onCopyMove(secret)}
            // A Move removes the source; it must never run while that secret's
            // edit/create draft is open (the draft would outlive its row).
            disabled={isBusy || showCreate || isEditing}
            className="min-h-11 cursor-pointer"
            aria-label={t("settings:copyMoveSecretNamed", { name: secret.name })}
          >
            <IconCopy className="h-4 w-4" />
            {t("settings:copyMove")}
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={handleReveal}
            disabled={revealing || isBusy}
            className="min-h-11 min-w-11 cursor-pointer"
            aria-label={
              revealed
                ? t("settings:hideSecretNamed", { name: secret.name })
                : t("settings:revealSecretNamed", { name: secret.name })
            }
          >
            {revealed ? <IconEyeOff className="h-4 w-4" /> : <IconEye className="h-4 w-4" />}
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => onEdit(secret)}
            disabled={isBusy || showCreate || isEditing}
            className="min-h-11 min-w-11 cursor-pointer"
            aria-label={t("settings:editSecretNamed", { name: secret.name })}
          >
            <IconEdit className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => onDelete(secret)}
            disabled={isBusy}
            className="min-h-11 min-w-11 cursor-pointer"
            aria-label={t("settings:deleteSecretNamed", { name: secret.name })}
          >
            <IconTrash className="h-4 w-4" />
          </Button>
        </div>
      </div>
      {revealed && revealedValue !== null && (
        <div className="text-xs font-mono bg-muted/50 rounded px-2 py-1 break-all">
          {revealedValue}
        </div>
      )}
    </div>
  );
}

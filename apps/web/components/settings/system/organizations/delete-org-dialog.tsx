"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import type { Org } from "@/lib/types/org";

type DeleteOrgDialogProps = {
  org: Org | null;
  busy: boolean;
  onClose: () => void;
  onConfirm: (org: Org) => void;
};

/**
 * Type-to-confirm deletion.
 *
 * Deleting an organization removes every workspace, task, session and account
 * inside it, so the operator types the slug verbatim. The comparison is
 * against the raw slug and is never translated: a localized token would be
 * impossible to type.
 */
export function DeleteOrgDialog({ org, busy, onClose, onConfirm }: DeleteOrgDialogProps) {
  const { t } = useTranslation();
  const [typed, setTyped] = useState("");

  useEffect(() => setTyped(""), [org?.id]);

  if (!org) return null;
  // i18n-exempt: compared with === against the server's slug; translating it
  // would make the confirmation impossible to satisfy.
  const matches = typed === org.slug;

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("orgs:deleteTitle", { name: org.name })}</DialogTitle>
          <DialogDescription>{t("orgs:deleteWarning")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="delete-org-confirm">
            {t("orgs:deleteConfirmLabel", { slug: org.slug })}
          </Label>
          <Input
            id="delete-org-confirm"
            value={typed}
            autoComplete="off"
            onChange={(event) => setTyped(event.target.value)}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" className="cursor-pointer" onClick={onClose}>
            {t("common:cancel")}
          </Button>
          <Button
            variant="destructive"
            className="cursor-pointer"
            disabled={!matches || busy}
            onClick={() => onConfirm(org)}
          >
            {t("orgs:deleteConfirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

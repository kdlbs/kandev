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

type OrgAdminDialogProps = {
  org: Org | null;
  busy: boolean;
  onClose: () => void;
  onCreate: (org: Org, admin: { email: string; password: string; display_name: string }) => void;
};

/**
 * Provisions an organization's first administrator.
 *
 * An ordinary admin can only create accounts in their own tenant, so a new
 * organization has no way to get its first user. This operator-only path is
 * what breaks that circularity; afterwards the org administers itself.
 */
export function OrgAdminDialog({ org, busy, onClose, onCreate }: OrgAdminDialogProps) {
  const { t } = useTranslation();
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");

  useEffect(() => {
    setEmail("");
    setDisplayName("");
    setPassword("");
  }, [org?.id]);

  if (!org) return null;
  const ready = email.trim() !== "" && password.length >= 8;

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("orgs:adminTitle", { name: org.name })}</DialogTitle>
          <DialogDescription>{t("orgs:adminDescription")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1">
            <Label htmlFor="org-admin-name">{t("orgs:adminName")}</Label>
            <Input
              id="org-admin-name"
              value={displayName}
              onChange={(event) => setDisplayName(event.target.value)}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="org-admin-email">{t("orgs:adminEmail")}</Label>
            <Input
              id="org-admin-email"
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="org-admin-password">{t("orgs:adminPassword")}</Label>
            <Input
              id="org-admin-password"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
            <p className="text-muted-foreground text-xs">{t("orgs:adminPasswordHint")}</p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" className="cursor-pointer" onClick={onClose}>
            {t("common:cancel")}
          </Button>
          <Button
            className="cursor-pointer"
            disabled={!ready || busy}
            onClick={() =>
              onCreate(org, { email: email.trim(), password, display_name: displayName.trim() })
            }
          >
            {t("orgs:adminCreate")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

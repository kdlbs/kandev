"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { IconCheck, IconCopy } from "@tabler/icons-react";
import { ApiError } from "@/lib/api/client";
import { createInvite } from "@/lib/api/domains/auth-api";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => void;
};

function InviteForm({
  email,
  setEmail,
  role,
  setRole,
  error,
  submitting,
  onCancel,
  onSubmit,
}: {
  email: string;
  setEmail: (v: string) => void;
  role: string;
  setRole: (v: string) => void;
  error: string | null;
  submitting: boolean;
  onCancel: () => void;
  onSubmit: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <DialogHeader>
        <DialogTitle>{t("system:inviteTitle")}</DialogTitle>
        <DialogDescription>{t("system:inviteDescription")}</DialogDescription>
      </DialogHeader>
      <div className="space-y-3">
        <div className="flex flex-col gap-1">
          <label htmlFor="invite-dialog-email" className="text-xs text-muted-foreground">
            {t("system:inviteEmailOptional")}
          </label>
          <Input
            id="invite-dialog-email"
            data-testid="invite-dialog-email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor="invite-dialog-role" className="text-xs text-muted-foreground">
            {t("system:createUserRole")}
          </label>
          <Select value={role} onValueChange={setRole}>
            <SelectTrigger id="invite-dialog-role" data-testid="invite-dialog-role">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {/* `value` is the wire role sent to the API; only the child text is copy. */}
              <SelectItem value="member">{t("system:usersRoleMember")}</SelectItem>
              <SelectItem value="admin">{t("system:usersRoleAdmin")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        {error && (
          <p className="text-xs text-destructive" data-testid="invite-dialog-error">
            {error}
          </p>
        )}
      </div>
      <DialogFooter>
        <Button variant="outline" className="cursor-pointer" onClick={onCancel}>
          {t("common:cancel")}
        </Button>
        <Button
          className="cursor-pointer"
          disabled={submitting}
          onClick={onSubmit}
          data-testid="invite-dialog-submit"
        >
          {submitting ? t("system:inviteCreating") : t("system:inviteCreate")}
        </Button>
      </DialogFooter>
    </>
  );
}

function InviteLinkResult({ url, onDone }: { url: string; onDone: () => void }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const onCopy = async () => {
    if (await copyToClipboard(url)) setCopied(true);
  };
  return (
    <>
      <DialogHeader>
        <DialogTitle>{t("system:inviteReadyTitle")}</DialogTitle>
        <DialogDescription>{t("system:inviteReadyDescription")}</DialogDescription>
      </DialogHeader>
      <div className="flex items-center gap-2">
        {/* The generated invite URL is a value. */}
        <Input
          readOnly
          value={url}
          aria-label={t("system:inviteUrlLabel")}
          data-testid="invite-dialog-url"
          className="font-mono text-xs"
        />
        <Button
          size="icon"
          variant="outline"
          className="cursor-pointer"
          onClick={() => void onCopy()}
          aria-label={copied ? t("system:inviteCopied") : t("system:inviteCopy")}
          data-testid="invite-dialog-copy"
        >
          {copied ? <IconCheck className="h-4 w-4" /> : <IconCopy className="h-4 w-4" />}
        </Button>
      </div>
      <DialogFooter>
        <Button className="cursor-pointer" onClick={onDone} data-dialog-default-action>
          {t("system:inviteDone")}
        </Button>
      </DialogFooter>
    </>
  );
}

export function InviteDialog({ open, onOpenChange, onCreated }: Props) {
  const { t } = useTranslation();
  const [email, setEmail] = useState("");
  const [role, setRole] = useState("member");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [resultUrl, setResultUrl] = useState<string | null>(null);

  const reset = () => {
    setEmail("");
    setRole("member");
    setError(null);
    setResultUrl(null);
  };

  const onSubmit = async () => {
    setError(null);
    setSubmitting(true);
    try {
      const { url } = await createInvite({ email: email || undefined, role });
      setResultUrl(`${window.location.origin}${url}`);
      onCreated();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("system:inviteFailed"));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset();
        onOpenChange(next);
      }}
    >
      <DialogContent data-testid="invite-dialog">
        {resultUrl ? (
          <InviteLinkResult url={resultUrl} onDone={() => onOpenChange(false)} />
        ) : (
          <InviteForm
            email={email}
            setEmail={setEmail}
            role={role}
            setRole={setRole}
            error={error}
            submitting={submitting}
            onCancel={() => onOpenChange(false)}
            onSubmit={() => void onSubmit()}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

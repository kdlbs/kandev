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
import { ApiError } from "@/lib/api/client";
import { createUser } from "@/lib/api/domains/auth-api";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => void;
};

type FormFields = {
  displayName: string;
  setDisplayName: (v: string) => void;
  email: string;
  setEmail: (v: string) => void;
  password: string;
  setPassword: (v: string) => void;
  role: string;
  setRole: (v: string) => void;
  error: string | null;
};

function CreateUserFields({
  displayName,
  setDisplayName,
  email,
  setEmail,
  password,
  setPassword,
  role,
  setRole,
  error,
}: FormFields) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <div className="flex flex-col gap-1">
        <label htmlFor="create-user-display-name" className="text-xs text-muted-foreground">
          {t("system:createUserDisplayName")}
        </label>
        <Input
          id="create-user-display-name"
          data-testid="create-user-display-name"
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
        />
      </div>
      <div className="flex flex-col gap-1">
        <label htmlFor="create-user-email" className="text-xs text-muted-foreground">
          {t("system:createUserEmail")}
        </label>
        <Input
          id="create-user-email"
          data-testid="create-user-email"
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
      </div>
      <div className="flex flex-col gap-1">
        <label htmlFor="create-user-password" className="text-xs text-muted-foreground">
          {t("system:createUserPassword")}
        </label>
        <Input
          id="create-user-password"
          data-testid="create-user-password"
          type="password"
          minLength={8}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
      </div>
      <div className="flex flex-col gap-1">
        <label htmlFor="create-user-role" className="text-xs text-muted-foreground">
          {t("system:createUserRole")}
        </label>
        <Select value={role} onValueChange={setRole}>
          <SelectTrigger id="create-user-role" data-testid="create-user-role">
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
        <p className="text-xs text-destructive" data-testid="create-user-error">
          {error}
        </p>
      )}
    </div>
  );
}

export function CreateUserDialog({ open, onOpenChange, onCreated }: Props) {
  const { t } = useTranslation();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [role, setRole] = useState("member");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const reset = () => {
    setEmail("");
    setPassword("");
    setDisplayName("");
    setRole("member");
    setError(null);
  };

  const onSubmit = async () => {
    setError(null);
    setSubmitting(true);
    try {
      await createUser({ email, password, display_name: displayName, role });
      onCreated();
      onOpenChange(false);
      reset();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("system:createUserFailed"));
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
      <DialogContent data-testid="create-user-dialog">
        <DialogHeader>
          <DialogTitle>{t("system:createUserTitle")}</DialogTitle>
          <DialogDescription>{t("system:createUserDescription")}</DialogDescription>
        </DialogHeader>
        <CreateUserFields
          displayName={displayName}
          setDisplayName={setDisplayName}
          email={email}
          setEmail={setEmail}
          password={password}
          setPassword={setPassword}
          role={role}
          setRole={setRole}
          error={error}
        />
        <DialogFooter>
          <Button variant="outline" className="cursor-pointer" onClick={() => onOpenChange(false)}>
            {t("common:cancel")}
          </Button>
          <Button
            className="cursor-pointer"
            disabled={submitting}
            onClick={() => void onSubmit()}
            data-testid="create-user-submit"
          >
            {submitting ? t("system:createUserSubmitting") : t("system:createUserSubmit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

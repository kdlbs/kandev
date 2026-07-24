"use client";

import { useCallback, useEffect, useState } from "react";
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
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { Switch } from "@kandev/ui/switch";
import { IconAlertTriangle, IconLock } from "@tabler/icons-react";
import { ApiError } from "@/lib/api/client";
import { fetchAuthSettings, updateAuthSettings, type AuthMode } from "@/lib/api/domains/auth-api";

function useAuthSettingsState() {
  const [mode, setMode] = useState<AuthMode | null>(null);
  const [envRequired, setEnvRequired] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetchAuthSettings({ cache: "no-store" });
      setMode(res.mode);
      setEnvRequired(res.env_required);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load authentication settings.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  return { mode, envRequired, loading, error, reload };
}

const MODE_LABELS: Record<AuthMode, string> = {
  enabled: "Enabled",
  setup: "Setup pending",
  disabled: "Disabled",
};

function ModeSummary({ mode }: { mode: AuthMode }) {
  const label = MODE_LABELS[mode];
  const variant = mode === "enabled" ? "default" : "secondary";
  return (
    <div className="flex items-center gap-2">
      <span className="text-sm">Current mode:</span>
      <Badge variant={variant} className="text-[10px]">
        {label}
      </Badge>
    </div>
  );
}

function DisableConfirmDialog({
  open,
  onOpenChange,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Disable authentication?</AlertDialogTitle>
          <AlertDialogDescription>
            Anyone who can reach this Kandev instance will get full access without signing in —
            existing users, sessions, and invites are kept but stop being enforced. You can
            re-enable authentication later; the first person to sign back in becomes an admin only
            if no admin account exists yet.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
            data-testid="authentication-settings-disable-confirm"
          >
            Disable authentication
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function AuthenticationSettings() {
  const { mode, envRequired, loading, error, reload } = useAuthSettingsState();
  const [confirmDisableOpen, setConfirmDisableOpen] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const applyEnabled = async (enabled: boolean) => {
    setActionError(null);
    setSaving(true);
    try {
      const res = await updateAuthSettings(enabled);
      if (res.setup_required) {
        window.location.assign("/setup");
        return;
      }
      await reload();
    } catch (err) {
      setActionError(err instanceof ApiError ? err.message : "Could not update this setting.");
    } finally {
      setSaving(false);
    }
  };

  const onSwitchChange = (checked: boolean) => {
    if (!checked) {
      setConfirmDisableOpen(true);
      return;
    }
    void applyEnabled(true);
  };

  return (
    <Card data-testid="authentication-settings-card">
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <IconLock className="h-4 w-4" /> Authentication
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-xs text-muted-foreground">
          When enabled, everyone who opens this Kandev instance must sign in. The first account
          created becomes an admin, who can invite or add further users from the Users page. Turning
          this off makes the instance open to anyone who can reach it.
        </p>
        {loading && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner className="size-4" /> Loading...
          </div>
        )}
        {!loading && error && (
          <p className="text-xs text-destructive" data-testid="authentication-settings-error">
            {error}
          </p>
        )}
        {!loading && mode && (
          <>
            <ModeSummary mode={mode} />
            {envRequired && (
              <p
                className="flex items-center gap-1.5 text-xs text-muted-foreground"
                data-testid="authentication-settings-env-banner"
              >
                <IconAlertTriangle className="h-3.5 w-3.5" /> Enforced by KANDEV_AUTH_REQUIRED —
                this cannot be disabled from settings while that environment variable is set.
              </p>
            )}
            <div className="flex items-center gap-2">
              <Switch
                checked={mode !== "disabled"}
                onCheckedChange={onSwitchChange}
                disabled={saving || (envRequired && mode !== "disabled")}
                data-testid="authentication-settings-switch"
              />
              <span className="text-sm">
                {mode === "disabled" ? "Enable authentication" : "Authentication is on"}
              </span>
            </div>
            {actionError && (
              <p
                className="text-xs text-destructive"
                data-testid="authentication-settings-action-error"
              >
                {actionError}
              </p>
            )}
          </>
        )}
      </CardContent>
      <DisableConfirmDialog
        open={confirmDisableOpen}
        onOpenChange={setConfirmDisableOpen}
        onConfirm={() => {
          setConfirmDisableOpen(false);
          void applyEnabled(false);
        }}
      />
    </Card>
  );
}

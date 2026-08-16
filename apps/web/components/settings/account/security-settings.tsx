"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Spinner } from "@kandev/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { Badge } from "@kandev/ui/badge";
import { IconDevices, IconKey } from "@tabler/icons-react";
import { ApiError } from "@/lib/api/client";
import {
  changePassword,
  listSessions,
  revokeSession,
  type AuthSession,
} from "@/lib/api/domains/auth-api";
import { SettingsCard } from "@/components/settings/settings-card";
import { ACCOUNT_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/account";
import { formatDateTime } from "@/lib/i18n/formats";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { SettingsErrorText, SettingsFieldLabel } from "@/components/settings/settings-typography";

function ChangePasswordCard() {
  const { t } = useTranslation();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setSuccess(false);
    setSubmitting(true);
    try {
      await changePassword({ current_password: current, new_password: next });
      setCurrent("");
      setNext("");
      setSuccess(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("account:couldNotChangePassword"));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <SettingsCard
      discoveryTargetId={ACCOUNT_SETTINGS_TARGETS.password}
      data-testid="account-security-password-card"
    >
      <SettingsCardHeader
        title={
          <span className="flex items-center gap-2">
            <IconKey className="h-4 w-4" /> {t("account:password")}
          </span>
        }
      />
      <CardContent>
        <form className="flex flex-col gap-3 max-w-sm" onSubmit={(e) => void onSubmit(e)}>
          <div className="flex flex-col gap-1">
            <SettingsFieldLabel htmlFor="account-current-password">
              {t("account:currentPassword")}
            </SettingsFieldLabel>
            <Input
              id="account-current-password"
              data-testid="account-current-password"
              type="password"
              value={current}
              onChange={(e) => setCurrent(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1">
            <SettingsFieldLabel htmlFor="account-new-password">
              {t("account:newPassword")}
            </SettingsFieldLabel>
            <Input
              id="account-new-password"
              data-testid="account-new-password"
              type="password"
              minLength={8}
              value={next}
              onChange={(e) => setNext(e.target.value)}
            />
          </div>
          {error && (
            <SettingsErrorText data-testid="account-password-error">{error}</SettingsErrorText>
          )}
          {success && (
            <p className="text-xs text-muted-foreground" data-testid="account-password-success">
              {t("account:passwordUpdated")}
            </p>
          )}
          <Button
            type="submit"
            className="cursor-pointer self-start"
            disabled={submitting}
            data-testid="account-password-submit"
          >
            {submitting ? t("account:saving") : t("account:changePassword")}
          </Button>
        </form>
      </CardContent>
    </SettingsCard>
  );
}

function useSessionsList() {
  const { t } = useTranslation();
  const [sessions, setSessions] = useState<AuthSession[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setError(null);
    try {
      const res = await listSessions({ cache: "no-store" });
      setSessions(res.sessions);
      setLoaded(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("account:failedToLoadSessions"));
    }
  }, [t]);

  useEffect(() => {
    void reload();
  }, [reload]);

  return { sessions, loaded, error, reload };
}

function SessionsCard() {
  const { t } = useTranslation();
  const { sessions, loaded, error, reload } = useSessionsList();

  const onRevoke = async (id: string) => {
    await revokeSession(id);
    await reload();
  };

  return (
    <SettingsCard
      discoveryTargetId={ACCOUNT_SETTINGS_TARGETS.sessions}
      data-testid="account-sessions-card"
    >
      <SettingsCardHeader
        title={
          <span className="flex items-center gap-2">
            <IconDevices className="h-4 w-4" /> {t("account:activeSessions")}
          </span>
        }
      />
      <CardContent className="space-y-3">
        {error && (
          <SettingsErrorText data-testid="account-sessions-error">{error}</SettingsErrorText>
        )}
        {!loaded && !error && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner className="size-4" /> {t("account:loadingSessions")}
          </div>
        )}
        {loaded && sessions.length > 0 && (
          <Table data-testid="account-sessions-table">
            <TableHeader>
              <TableRow>
                <TableHead>{t("account:device")}</TableHead>
                <TableHead>{t("account:ipAddress")}</TableHead>
                <TableHead>{t("account:lastSeen")}</TableHead>
                <TableHead className="text-right">{t("account:actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sessions.map((session) => (
                <TableRow key={session.id} data-testid="account-sessions-row">
                  <TableCell className="text-xs">
                    {session.user_agent}
                    {session.current && (
                      <Badge variant="default" className="ml-2 text-[10px]">
                        {t("account:thisDevice")}
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell className="text-xs">{session.ip}</TableCell>
                  <TableCell className="text-xs">{formatDateTime(session.last_seen_at)}</TableCell>
                  <TableCell className="text-right">
                    {!session.current && (
                      <Button
                        size="sm"
                        variant="ghost"
                        className="cursor-pointer text-destructive"
                        onClick={() => void onRevoke(session.id)}
                        data-testid="account-sessions-revoke"
                      >
                        {t("account:signOut")}
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </SettingsCard>
  );
}

export function SecuritySettings() {
  return (
    <div className="space-y-4">
      <ChangePasswordCard />
      <SessionsCard />
    </div>
  );
}

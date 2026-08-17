"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Spinner } from "@kandev/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
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
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useSettingsTargetRegistration } from "@/components/settings/settings-target-provider";
import { ACCOUNT_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/account";
import { formatDateTime, formatRelativeTime } from "@/lib/i18n/formats";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { SettingsErrorText, SettingsFieldLabel } from "@/components/settings/settings-typography";
import { useNow } from "@/hooks/use-now";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { createQueuedUserSettingsSyncWithResponse } from "@/lib/user-settings-sync";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import { toast } from "@/lib/toast/sonner";
import type { LastSeenDisplay } from "@/lib/types/http";

/** Queued sync that persists the last-seen display preference to the backend. */
const syncLastSeenDisplay = createQueuedUserSettingsSyncWithResponse<LastSeenDisplay>(
  (lastSeenDisplay) => ({ last_seen_display: lastSeenDisplay }),
);

/** Renders the change-password form card. */
function ChangePasswordCard() {
  const { t } = useTranslation();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  /** Submits the password change and updates success/error state. */
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

/** Loads the active sessions list and exposes reload/error state. */
function useSessionsList() {
  const { t } = useTranslation();
  const [sessions, setSessions] = useState<AuthSession[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);

  /** Fetches the latest sessions from the API and updates state. */
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

  return { sessions, loaded, error, reload, setError };
}

/** Parses a last-seen timestamp into a Date, or null when invalid. */
function parseLastSeenAt(lastSeenAt: string): Date | null {
  const parsed = new Date(lastSeenAt);
  return Number.isNaN(parsed.getTime()) ? null : parsed;
}

/** Renders a session's last-seen timestamp in the selected display format. */
function LastSeenCell({ lastSeenAt, display }: { lastSeenAt: string; display: LastSeenDisplay }) {
  const absolute = useMemo(() => parseLastSeenAt(lastSeenAt), [lastSeenAt]);

  if (!absolute) {
    // Guard formatDateTime, which throws RangeError on invalid dates.
    return <TableCell className="text-xs" data-testid="last-seen-empty" />;
  }
  if (display === "absolute") {
    return <TableCell className="text-xs">{formatDateTime(absolute)}</TableCell>;
  }
  return <RelativeLastSeenCell lastSeenAt={absolute} />;
}

/** Renders a live-updating relative last-seen time with an absolute-time tooltip. */
function RelativeLastSeenCell({ lastSeenAt }: { lastSeenAt: Date }) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  // Tick every second while the timestamp is under a minute old (second-scale
  // labels like "45 seconds ago" need live updates), then every minute once
  // the age crosses a minute. The interval is recomputed on each render, so
  // useNow re-creates its timer exactly when the cadence changes.
  const ageMs = Math.abs(Date.now() - lastSeenAt.getTime());
  const intervalMs = ageMs < 60_000 ? 1_000 : 60_000;
  const now = useNow(intervalMs);
  const absolute = formatDateTime(lastSeenAt);
  const relative = formatRelativeTime(lastSeenAt, now);

  if (usesTouchDrawer) {
    return (
      <TableCell className="text-xs">
        <Drawer open={open} onOpenChange={setOpen}>
          <DrawerTrigger asChild>
            <button
              type="button"
              aria-label={absolute}
              title={absolute}
              aria-haspopup="dialog"
              aria-expanded={open}
              className="min-h-11 cursor-pointer rounded-sm px-1 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
              data-testid="last-seen-relative"
            >
              <time dateTime={lastSeenAt.toISOString()}>{relative}</time>
            </button>
          </DrawerTrigger>
          <DrawerContent>
            <DrawerHeader>
              <DrawerTitle>{t("account:lastSeen")}</DrawerTitle>
              <DrawerDescription data-testid="last-seen-absolute">{absolute}</DrawerDescription>
            </DrawerHeader>
          </DrawerContent>
        </Drawer>
      </TableCell>
    );
  }

  return (
    <TableCell className="text-xs">
      <Tooltip>
        <TooltipTrigger asChild>
          <time
            dateTime={lastSeenAt.toISOString()}
            tabIndex={0}
            title={absolute}
            aria-label={absolute}
            className="cursor-default rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
            data-testid="last-seen-relative"
          >
            {relative}
          </time>
        </TooltipTrigger>
        <TooltipContent>{absolute}</TooltipContent>
      </Tooltip>
    </TableCell>
  );
}

/** Renders the last-seen display format preference selector. */
function LastSeenDisplaySelect({
  value,
  onChange,
}: {
  value: LastSeenDisplay;
  onChange: (value: LastSeenDisplay) => void;
}) {
  const { t } = useTranslation();
  /** Ref callback that registers this element as the last-seen display discovery target. */
  const registerTarget = useSettingsTargetRegistration(ACCOUNT_SETTINGS_TARGETS.lastSeenDisplay);
  return (
    <div ref={registerTarget} className="space-y-2" data-testid="last-seen-display-control">
      <Label htmlFor="last-seen-display">{t("account:lastSeenDisplay")}</Label>
      <Select value={value} onValueChange={(next) => onChange(next as LastSeenDisplay)}>
        <SelectTrigger
          id="last-seen-display"
          data-testid="last-seen-display-select"
          className="min-h-[44px] cursor-pointer"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="absolute" className="min-h-[44px]">
            {t("account:absoluteTime")}
          </SelectItem>
          <SelectItem value="relative" className="min-h-[44px]">
            {t("account:relativeTime")}
          </SelectItem>
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">{t("account:lastSeenDisplayDescription")}</p>
    </div>
  );
}

/** Renders the active sessions table with revoke actions. */
function SessionsTable({
  sessions,
  display,
  onRevoke,
}: {
  sessions: AuthSession[];
  display: LastSeenDisplay;
  onRevoke: (id: string) => void;
}) {
  const { t } = useTranslation();
  return (
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
          <TableRow
            key={session.id}
            data-testid="account-sessions-row"
            data-current-session={session.current ? "true" : "false"}
          >
            <TableCell className="text-xs">
              {session.user_agent}
              {session.current && (
                <Badge variant="default" className="ml-2 text-[10px]">
                  {t("account:thisDevice")}
                </Badge>
              )}
            </TableCell>
            <TableCell className="text-xs">{session.ip}</TableCell>
            <LastSeenCell lastSeenAt={session.last_seen_at} display={display} />
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
  );
}

/** Renders the active-sessions settings card with display preference and revoke. */
function SessionsCard() {
  const { t } = useTranslation();
  const { sessions, loaded, error, reload, setError } = useSessionsList();
  const savedDisplay = useAppStore((state) => state.userSettings.lastSeenDisplay);
  const store = useAppStoreApi();
  const [draftDisplay, setDraftDisplay] = useState<LastSeenDisplay>(savedDisplay);
  const hasLocalDraft = useRef(false);
  const isDirty = draftDisplay !== savedDisplay;

  useEffect(() => {
    if (!hasLocalDraft.current) setDraftDisplay(savedDisplay);
  }, [savedDisplay]);

  const onDisplayChange = useCallback((next: LastSeenDisplay) => {
    hasLocalDraft.current = true;
    setDraftDisplay(next);
  }, []);

  useSettingsSaveContributor({
    id: "account-last-seen-display",
    order: 30,
    revision: draftDisplay,
    isDirty,
    save: async (revision) => {
      const submitted = revision as LastSeenDisplay;
      try {
        const response = await syncLastSeenDisplay(submitted);
        const state = store.getState();
        state.setUserSettings(mapUserSettingsResponse(response, state.userSettings));
        const confirmed = store.getState().userSettings.lastSeenDisplay;
        setDraftDisplay((current) => {
          if (current !== submitted) return current;
          hasLocalDraft.current = false;
          return confirmed;
        });
      } catch (error) {
        toast.error(t("account:failedToSaveLastSeenDisplay"));
        throw error;
      }
    },
    discard: () => {
      hasLocalDraft.current = false;
      setDraftDisplay(savedDisplay);
    },
  });

  /** Revokes a session and reloads the sessions list. */
  const onRevoke = async (id: string) => {
    try {
      await revokeSession(id);
      await reload();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("account:failedToRevokeSession"));
    }
  };

  return (
    <SettingsCard
      discoveryTargetId={ACCOUNT_SETTINGS_TARGETS.sessions}
      isDirty={isDirty}
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
        {/* The display preference is independent of session rows, so the
            select (and its discovery target) stays reachable during loading
            and when the sessions API fails. */}
        <LastSeenDisplaySelect value={draftDisplay} onChange={onDisplayChange} />
        {error && (
          <SettingsErrorText data-testid="account-sessions-error">{error}</SettingsErrorText>
        )}
        {!loaded && !error && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner className="size-4" /> {t("account:loadingSessions")}
          </div>
        )}
        {loaded && sessions.length > 0 && (
          <SessionsTable sessions={sessions} display={draftDisplay} onRevoke={onRevoke} />
        )}
      </CardContent>
    </SettingsCard>
  );
}

/** Renders the account security settings section. */
export function SecuritySettings() {
  return (
    <div className="space-y-4">
      <ChangePasswordCard />
      <SessionsCard />
    </div>
  );
}

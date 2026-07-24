"use client";

import { useCallback, useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
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

function ChangePasswordCard() {
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
      setError(err instanceof ApiError ? err.message : "Could not change password.");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Card data-testid="account-security-password-card">
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <IconKey className="h-4 w-4" /> Password
        </CardTitle>
      </CardHeader>
      <CardContent>
        <form className="flex flex-col gap-3 max-w-sm" onSubmit={(e) => void onSubmit(e)}>
          <div className="flex flex-col gap-1">
            <label htmlFor="account-current-password" className="text-xs text-muted-foreground">
              Current password
            </label>
            <Input
              id="account-current-password"
              data-testid="account-current-password"
              type="password"
              value={current}
              onChange={(e) => setCurrent(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1">
            <label htmlFor="account-new-password" className="text-xs text-muted-foreground">
              New password
            </label>
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
            <p className="text-xs text-destructive" data-testid="account-password-error">
              {error}
            </p>
          )}
          {success && (
            <p className="text-xs text-muted-foreground" data-testid="account-password-success">
              Password updated.
            </p>
          )}
          <Button
            type="submit"
            className="cursor-pointer self-start"
            disabled={submitting}
            data-testid="account-password-submit"
          >
            {submitting ? "Saving..." : "Change password"}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

function useSessionsList() {
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
      setError(err instanceof ApiError ? err.message : "Failed to load sessions.");
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  return { sessions, loaded, error, reload };
}

function SessionsCard() {
  const { sessions, loaded, error, reload } = useSessionsList();

  const onRevoke = async (id: string) => {
    await revokeSession(id);
    await reload();
  };

  return (
    <Card data-testid="account-sessions-card">
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <IconDevices className="h-4 w-4" /> Active sessions
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {error && (
          <p className="text-xs text-destructive" data-testid="account-sessions-error">
            {error}
          </p>
        )}
        {!loaded && !error && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner className="size-4" /> Loading sessions...
          </div>
        )}
        {loaded && sessions.length > 0 && (
          <Table data-testid="account-sessions-table">
            <TableHeader>
              <TableRow>
                <TableHead>Device</TableHead>
                <TableHead>IP</TableHead>
                <TableHead>Last seen</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sessions.map((session) => (
                <TableRow key={session.id} data-testid="account-sessions-row">
                  <TableCell className="text-xs">
                    {session.user_agent}
                    {session.current && (
                      <Badge variant="default" className="ml-2 text-[10px]">
                        This device
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell className="text-xs">{session.ip}</TableCell>
                  <TableCell className="text-xs">
                    {new Date(session.last_seen_at).toLocaleString()}
                  </TableCell>
                  <TableCell className="text-right">
                    {!session.current && (
                      <Button
                        size="sm"
                        variant="ghost"
                        className="cursor-pointer text-destructive"
                        onClick={() => void onRevoke(session.id)}
                        data-testid="account-sessions-revoke"
                      >
                        Sign out
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
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

"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { fetchGitHubCLIAccounts } from "@/lib/api/domains/github-api";
import type { GitHubCLIAccount } from "@/lib/types/github";

function GitHubCLIAccountNotice({
  loadError,
  loading,
  hasAccounts,
}: {
  loadError: string | null;
  loading: boolean;
  hasAccounts: boolean;
}) {
  if (loading || hasAccounts) return null;
  if (loadError) {
    return (
      <p role="alert" className="text-xs text-destructive">
        Could not load GitHub CLI accounts. {loadError}
      </p>
    );
  }
  return (
    <p className="text-xs text-muted-foreground">
      Sign in with <code>gh auth login</code>, then reopen this dialog.
    </p>
  );
}

function accountPlaceholder(loading: boolean, loadError: string | null) {
  if (loading) return "Loading accounts...";
  if (loadError) return "Accounts unavailable";
  return "No gh accounts found";
}

export function GitHubCLIForm({
  workspaceId,
  onAccountChange,
  disabled,
}: {
  workspaceId: string;
  onAccountChange: (account: GitHubCLIAccount | null) => void;
  disabled?: boolean;
}) {
  const [accounts, setAccounts] = useState<GitHubCLIAccount[]>([]);
  const [selected, setSelected] = useState("");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const onAccountChangeRef = useRef(onAccountChange);
  onAccountChangeRef.current = onAccountChange;

  useEffect(() => {
    let current = true;
    setAccounts([]);
    setSelected("");
    setLoadError(null);
    setLoading(true);
    onAccountChangeRef.current(null);
    fetchGitHubCLIAccounts(workspaceId, { cache: "no-store" })
      .then((items) => {
        if (!current) return;
        setAccounts(items);
        const preferred =
          items.find((item) => item.selected) ?? items.find((item) => item.active) ?? items[0];
        setSelected(preferred ? `${preferred.host}\n${preferred.login}` : "");
      })
      .catch((error) => {
        if (!current) return;
        setAccounts([]);
        setLoadError(
          error instanceof Error ? error.message : "Unexpected response from the server",
        );
      })
      .finally(() => current && setLoading(false));
    return () => {
      current = false;
    };
  }, [workspaceId]);

  const account = useMemo(() => {
    const [host, login] = selected.split("\n");
    return accounts.find((item) => item.host === host && item.login === login);
  }, [accounts, selected]);

  useEffect(() => {
    onAccountChangeRef.current(account ?? null);
  }, [account]);

  return (
    <div className="space-y-3">
      <div className="space-y-1">
        <Label htmlFor="github-cli-account">GitHub CLI account</Label>
        <p className="text-xs text-muted-foreground">
          Choose the exact account. Kandev does not silently follow the CLI's active-account change.
        </p>
      </div>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-stretch">
        <Select
          value={selected}
          onValueChange={setSelected}
          disabled={disabled || loading || !accounts.length}
        >
          <SelectTrigger id="github-cli-account" className="min-h-11 min-w-0 flex-1">
            <SelectValue placeholder={accountPlaceholder(loading, loadError)} />
          </SelectTrigger>
          <SelectContent>
            {accounts.map((item) => (
              <SelectItem key={`${item.host}:${item.login}`} value={`${item.host}\n${item.login}`}>
                {item.login} ({item.host}){item.active ? " - active" : ""}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <GitHubCLIAccountNotice
        loadError={loadError}
        loading={loading}
        hasAccounts={accounts.length > 0}
      />
    </div>
  );
}

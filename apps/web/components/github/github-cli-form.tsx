"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Spinner } from "@kandev/ui/spinner";
import { useToast } from "@/components/toast-provider";
import { fetchGitHubCLIAccounts, setGitHubWorkspaceConnection } from "@/lib/api/domains/github-api";
import type { GitHubCLIAccount } from "@/lib/types/github";

export function GitHubCLIForm({
  workspaceId,
  onSaved,
}: {
  workspaceId: string;
  onSaved: () => void;
}) {
  const [accounts, setAccounts] = useState<GitHubCLIAccount[]>([]);
  const [selected, setSelected] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const { toast } = useToast();

  useEffect(() => {
    let current = true;
    setAccounts([]);
    setSelected("");
    setLoading(true);
    setSaving(false);
    fetchGitHubCLIAccounts(workspaceId, { cache: "no-store" })
      .then((items) => {
        if (!current) return;
        setAccounts(items);
        const preferred =
          items.find((item) => item.selected) ?? items.find((item) => item.active) ?? items[0];
        setSelected(preferred ? `${preferred.host}\n${preferred.login}` : "");
      })
      .catch(() => current && setAccounts([]))
      .finally(() => current && setLoading(false));
    return () => {
      current = false;
    };
  }, [workspaceId]);

  const account = useMemo(() => {
    const [host, login] = selected.split("\n");
    return accounts.find((item) => item.host === host && item.login === login);
  }, [accounts, selected]);

  const connect = useCallback(async () => {
    if (!account) return;
    setSaving(true);
    try {
      await setGitHubWorkspaceConnection(workspaceId, {
        source: "gh_cli",
        host: account.host,
        login: account.login,
      });
      toast({ description: `Connected ${account.login} for this workspace`, variant: "success" });
      onSaved();
    } catch (error) {
      toast({
        description: error instanceof Error ? error.message : "Connection failed",
        variant: "error",
      });
    } finally {
      setSaving(false);
    }
  }, [account, onSaved, toast, workspaceId]);

  return (
    <div className="space-y-3">
      <div className="space-y-1">
        <Label htmlFor="github-cli-account">GitHub CLI account</Label>
        <p className="text-xs text-muted-foreground">
          Choose the exact account. Kandev does not silently follow the CLI's active-account change.
        </p>
      </div>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-stretch">
        <Select value={selected} onValueChange={setSelected} disabled={loading || !accounts.length}>
          <SelectTrigger id="github-cli-account" className="min-h-11 min-w-0 flex-1">
            <SelectValue placeholder={loading ? "Loading accounts..." : "No gh accounts found"} />
          </SelectTrigger>
          <SelectContent>
            {accounts.map((item) => (
              <SelectItem key={`${item.host}:${item.login}`} value={`${item.host}\n${item.login}`}>
                {item.login} ({item.host}){item.active ? " - active" : ""}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button onClick={connect} disabled={!account || saving} className="h-11 cursor-pointer">
          {saving && <Spinner className="mr-2 h-4 w-4" />}
          Use account
        </Button>
      </div>
      {!accounts.length && !loading && (
        <p className="text-xs text-muted-foreground">
          Sign in with <code>gh auth login</code>, then reopen this dialog.
        </p>
      )}
    </div>
  );
}

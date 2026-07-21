"use client";

import { useCallback, useEffect, useState } from "react";
import { IconEye, IconEyeOff } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Spinner } from "@kandev/ui/spinner";
import { useToast } from "@/components/toast-provider";
import { setGitHubWorkspaceConnection } from "@/lib/api/domains/github-api";

export function GitHubPATForm({
  workspaceId,
  onSaved,
}: {
  workspaceId: string;
  onSaved: () => void;
}) {
  const [token, setToken] = useState("");
  const [visible, setVisible] = useState(false);
  const [saving, setSaving] = useState(false);
  const { toast } = useToast();

  useEffect(() => {
    setToken("");
    setVisible(false);
    setSaving(false);
  }, [workspaceId]);

  const submit = useCallback(
    async (event: React.FormEvent) => {
      event.preventDefault();
      if (!token.trim()) return;
      setSaving(true);
      try {
        await setGitHubWorkspaceConnection(workspaceId, { source: "pat", token: token.trim() });
        setToken("");
        toast({ description: "Workspace GitHub token connected", variant: "success" });
        onSaved();
      } catch (error) {
        toast({
          description: error instanceof Error ? error.message : "Connection failed",
          variant: "error",
        });
      } finally {
        setSaving(false);
      }
    },
    [onSaved, toast, token, workspaceId],
  );

  return (
    <form onSubmit={submit} className="space-y-3">
      <div className="space-y-1">
        <Label htmlFor="github-workspace-token">Personal access token</Label>
        <p className="text-xs text-muted-foreground">
          Kandev stores this token for this workspace. GitHub records actions as the token owner.
        </p>
      </div>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-stretch">
        <div className="relative min-w-0 flex-1">
          <Input
            id="github-workspace-token"
            type={visible ? "text" : "password"}
            value={token}
            onChange={(event) => setToken(event.target.value)}
            placeholder="ghp_xxxxxxxxxxxx"
            autoComplete="off"
            className="h-11 pr-11 font-mono"
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="absolute right-0 top-0 h-11 w-11 cursor-pointer"
            onClick={() => setVisible((current) => !current)}
            aria-label={visible ? "Hide token" : "Show token"}
          >
            {visible ? <IconEyeOff className="h-4 w-4" /> : <IconEye className="h-4 w-4" />}
          </Button>
        </div>
        <Button type="submit" disabled={!token.trim() || saving} className="h-11 cursor-pointer">
          {saving && <Spinner className="mr-2 h-4 w-4" />}
          Connect token
        </Button>
      </div>
    </form>
  );
}

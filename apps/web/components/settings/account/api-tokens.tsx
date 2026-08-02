"use client";

import { useCallback, useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Spinner } from "@kandev/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { IconCheck, IconCopy, IconKey } from "@tabler/icons-react";
import { ApiError } from "@/lib/api/client";
import { listTokens, mintToken, revokeToken, type ApiToken } from "@/lib/api/domains/auth-api";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";

function useTokensList() {
  const [tokens, setTokens] = useState<ApiToken[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setError(null);
    try {
      const res = await listTokens({ cache: "no-store" });
      setTokens(res.tokens);
      setLoaded(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load tokens.");
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  return { tokens, loaded, error, reload };
}

function MintTokenResult({
  rawToken,
  copied,
  onCopy,
  onDone,
}: {
  rawToken: string;
  copied: boolean;
  onCopy: () => void;
  onDone: () => void;
}) {
  return (
    <>
      <DialogHeader>
        <DialogTitle>Token created</DialogTitle>
        <DialogDescription>
          This is shown only once — copy it now and store it securely.
        </DialogDescription>
      </DialogHeader>
      <div className="flex items-center gap-2">
        <Input
          readOnly
          value={rawToken}
          className="font-mono text-xs"
          data-testid="api-tokens-raw-value"
        />
        <Button
          size="icon"
          variant="outline"
          className="cursor-pointer"
          onClick={onCopy}
          data-testid="api-tokens-copy"
        >
          {copied ? <IconCheck className="h-4 w-4" /> : <IconCopy className="h-4 w-4" />}
        </Button>
      </div>
      <DialogFooter>
        <Button className="cursor-pointer" onClick={onDone} data-dialog-default-action>
          Done
        </Button>
      </DialogFooter>
    </>
  );
}

function MintTokenForm({
  name,
  setName,
  error,
  submitting,
  onCancel,
  onSubmit,
}: {
  name: string;
  setName: (v: string) => void;
  error: string | null;
  submitting: boolean;
  onCancel: () => void;
  onSubmit: () => void;
}) {
  return (
    <>
      <DialogHeader>
        <DialogTitle>Create API token</DialogTitle>
        <DialogDescription>
          Grants the same access as your account to any script or CLI that uses it. Give it a name
          so you can recognize it later.
        </DialogDescription>
      </DialogHeader>
      <div className="flex flex-col gap-1">
        <label htmlFor="api-tokens-name" className="text-xs text-muted-foreground">
          Name
        </label>
        <Input
          id="api-tokens-name"
          data-testid="api-tokens-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </div>
      {error && (
        <p className="text-xs text-destructive" data-testid="api-tokens-mint-error">
          {error}
        </p>
      )}
      <DialogFooter>
        <Button variant="outline" className="cursor-pointer" onClick={onCancel}>
          Cancel
        </Button>
        <Button
          className="cursor-pointer"
          disabled={submitting || !name}
          onClick={onSubmit}
          data-testid="api-tokens-mint-submit"
        >
          {submitting ? "Creating..." : "Create token"}
        </Button>
      </DialogFooter>
    </>
  );
}

function MintTokenDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => void;
}) {
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [rawToken, setRawToken] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const reset = () => {
    setName("");
    setError(null);
    setRawToken(null);
    setCopied(false);
  };

  const onSubmit = async () => {
    setError(null);
    setSubmitting(true);
    try {
      const res = await mintToken({ name });
      setRawToken(res.token);
      onCreated();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not create token.");
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
      <DialogContent data-testid="api-tokens-mint-dialog">
        {rawToken ? (
          <MintTokenResult
            rawToken={rawToken}
            copied={copied}
            onCopy={() => {
              void copyToClipboard(rawToken).then((success) => {
                if (success) setCopied(true);
              });
            }}
            onDone={() => onOpenChange(false)}
          />
        ) : (
          <MintTokenForm
            name={name}
            setName={setName}
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

export function ApiTokens() {
  const { tokens, loaded, error, reload } = useTokensList();
  const [mintOpen, setMintOpen] = useState(false);

  const onRevoke = async (id: string) => {
    await revokeToken(id);
    await reload();
  };

  return (
    <Card data-testid="api-tokens-card">
      <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
        <CardTitle className="text-base flex items-center gap-2">
          <IconKey className="h-4 w-4" /> API tokens
        </CardTitle>
        <Button
          size="sm"
          className="cursor-pointer"
          onClick={() => setMintOpen(true)}
          data-testid="api-tokens-create"
        >
          New token
        </Button>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-xs text-muted-foreground">
          Personal access tokens authenticate scripts and CLIs as you. Revoke a token immediately if
          it may have leaked.
        </p>
        {error && (
          <p className="text-xs text-destructive" data-testid="api-tokens-error">
            {error}
          </p>
        )}
        {!loaded && !error && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner className="size-4" /> Loading tokens...
          </div>
        )}
        {loaded && tokens.length > 0 && (
          <Table data-testid="api-tokens-table">
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Last used</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokens.map((token) => (
                <TableRow key={token.id} data-testid="api-tokens-row">
                  <TableCell className="text-xs">{token.name}</TableCell>
                  <TableCell className="text-xs">
                    {new Date(token.created_at).toLocaleString()}
                  </TableCell>
                  <TableCell className="text-xs">
                    {token.last_used_at ? new Date(token.last_used_at).toLocaleString() : "Never"}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      size="sm"
                      variant="ghost"
                      className="cursor-pointer text-destructive"
                      onClick={() => void onRevoke(token.id)}
                      data-testid="api-tokens-revoke"
                    >
                      Revoke
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        {loaded && tokens.length === 0 && !error && (
          <p className="text-sm text-muted-foreground" data-testid="api-tokens-empty">
            No tokens yet.
          </p>
        )}
      </CardContent>
      <MintTokenDialog open={mintOpen} onOpenChange={setMintOpen} onCreated={() => void reload()} />
    </Card>
  );
}

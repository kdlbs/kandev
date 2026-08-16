"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
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
import { formatDateTime } from "@/lib/i18n/formats";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import {
  SettingsErrorText,
  SettingsFieldDescription,
  SettingsFieldLabel,
} from "@/components/settings/settings-typography";
import { settingsActionClassName } from "@/components/settings/settings-control";

function useTokensList() {
  const { t } = useTranslation();
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
      setError(err instanceof ApiError ? err.message : t("account:failedToLoadTokens"));
    }
  }, [t]);

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
  const { t } = useTranslation();
  return (
    <>
      <DialogHeader>
        <DialogTitle>{t("account:tokenCreated")}</DialogTitle>
        <DialogDescription>{t("account:tokenShownOnlyOnce")}</DialogDescription>
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
          {t("account:done")}
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
  const { t } = useTranslation();
  return (
    <>
      <DialogHeader>
        <DialogTitle>{t("account:createApiToken")}</DialogTitle>
        <DialogDescription>{t("account:createApiTokenDescription")}</DialogDescription>
      </DialogHeader>
      <div className="flex flex-col gap-1">
        <SettingsFieldLabel htmlFor="api-tokens-name">{t("account:name")}</SettingsFieldLabel>
        <Input
          id="api-tokens-name"
          data-testid="api-tokens-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </div>
      {error && <SettingsErrorText data-testid="api-tokens-mint-error">{error}</SettingsErrorText>}
      <DialogFooter>
        <Button variant="outline" className="cursor-pointer" onClick={onCancel}>
          {t("account:cancel")}
        </Button>
        <Button
          className="cursor-pointer"
          disabled={submitting || !name}
          onClick={onSubmit}
          data-testid="api-tokens-mint-submit"
        >
          {submitting ? t("account:creating") : t("account:createToken")}
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
  const { t } = useTranslation();
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
      setError(err instanceof ApiError ? err.message : t("account:couldNotCreateToken"));
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
  const { t } = useTranslation();
  const { tokens, loaded, error, reload } = useTokensList();
  const [mintOpen, setMintOpen] = useState(false);

  const onRevoke = async (id: string) => {
    await revokeToken(id);
    await reload();
  };

  return (
    <Card data-testid="api-tokens-card">
      <SettingsCardHeader
        title={
          <span className="flex items-center gap-2">
            <IconKey className="h-4 w-4" /> {t("account:apiTokens")}
          </span>
        }
        actions={
          <Button
            size="sm"
            className={settingsActionClassName("cursor-pointer")}
            onClick={() => setMintOpen(true)}
            data-testid="api-tokens-create"
          >
            {t("account:newToken")}
          </Button>
        }
      />
      <CardContent className="space-y-3">
        <SettingsFieldDescription>{t("account:apiTokensBlurb")}</SettingsFieldDescription>
        {error && <SettingsErrorText data-testid="api-tokens-error">{error}</SettingsErrorText>}
        {!loaded && !error && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner className="size-4" /> {t("account:loadingTokens")}
          </div>
        )}
        {loaded && tokens.length > 0 && (
          <Table data-testid="api-tokens-table">
            <TableHeader>
              <TableRow>
                <TableHead>{t("account:name")}</TableHead>
                <TableHead>{t("account:created")}</TableHead>
                <TableHead>{t("account:lastUsed")}</TableHead>
                <TableHead className="text-right">{t("account:actions")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokens.map((token) => (
                <TableRow key={token.id} data-testid="api-tokens-row">
                  <TableCell className="text-sm">{token.name}</TableCell>
                  <TableCell className="text-xs">{formatDateTime(token.created_at)}</TableCell>
                  <TableCell className="text-xs">
                    {token.last_used_at ? formatDateTime(token.last_used_at) : t("account:never")}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      size="sm"
                      variant="ghost"
                      className="cursor-pointer text-destructive"
                      onClick={() => void onRevoke(token.id)}
                      data-testid="api-tokens-revoke"
                    >
                      {t("account:revoke")}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        {loaded && tokens.length === 0 && !error && (
          <p className="text-sm text-muted-foreground" data-testid="api-tokens-empty">
            {t("account:noTokensYet")}
          </p>
        )}
      </CardContent>
      <MintTokenDialog open={mintOpen} onOpenChange={setMintOpen} onCreated={() => void reload()} />
    </Card>
  );
}

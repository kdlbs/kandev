"use client";

import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
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
import { Badge } from "@kandev/ui/badge";
import { IconRefresh, IconSearch } from "@tabler/icons-react";
import { marketplaceInputsComplete } from "@/components/mcp/mcp-config-inputs";
import { MCPMarketplaceInputs } from "./mcp-marketplace-inputs";
import type {
  MCPMarketplaceEntry,
  MCPMarketplaceChoice,
  MCPMarketplaceInstallInput,
  MCPSecretBinding,
} from "@/lib/types/http-mcp";

type MCPMarketplaceProps = {
  workspaceId: string;
  entries: MCPMarketplaceEntry[];
  stale: boolean;
  degraded: boolean;
  loading: boolean;
  onSearch: (query: string) => Promise<void>;
  onRefresh: () => Promise<void>;
  onInstall: (payload: MCPMarketplaceInstallInput) => Promise<void>;
};

function choiceLabel(choice: MCPMarketplaceChoice, t: (key: string) => string): string {
  if (choice.kind === "package")
    return `${choice.identifier ?? t("settings:unknownPackage")}@${choice.version ?? ""}`;
  return choice.url ?? t("settings:remoteEndpointUnavailable");
}

function choiceAvailability(choice: MCPMarketplaceChoice, t: (key: string) => string) {
  if (choice.selectable) return t("settings:mcpChoiceSupported");
  if (choice.unsupported_reason === "input_arguments_unsupported")
    return t("settings:mcpUnsupportedArguments");
  if (choice.unsupported_reason === "input_variables_unsupported")
    return t("settings:mcpUnsupportedVariables");
  return choice.unsupported_reason;
}

function statusLabel(status: string, t: (key: string) => string): string {
  if (status === "deprecated") return t("settings:mcpStatusDeprecated");
  if (status === "deleted") return t("settings:mcpStatusDeleted");
  return t("settings:mcpStatusActive");
}

function MarketplaceSearchControls({
  query,
  loading,
  onQueryChange,
  onSearch,
  onRefresh,
}: {
  query: string;
  loading: boolean;
  onQueryChange: (query: string) => void;
  onSearch: () => void;
  onRefresh: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-3 sm:flex-row">
      <div className="relative min-w-0 flex-1">
        <IconSearch className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          value={query}
          onChange={(event) => onQueryChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") onSearch();
          }}
          placeholder={t("settings:searchMcpMarketplace")}
          className="pl-9"
          aria-label={t("settings:searchMcpMarketplace")}
        />
      </div>
      <Button
        type="button"
        variant="outline"
        className={controlSizingClassName("standard", "cursor-pointer")}
        onClick={onRefresh}
        disabled={loading}
      >
        <IconRefresh className="mr-2 size-4" />
        {t("settings:refreshMarketplace")}
      </Button>
    </div>
  );
}

function MarketplaceEntryCard({
  entry,
  onReview,
}: {
  entry: MCPMarketplaceEntry;
  onReview: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Card key={`${entry.source}:${entry.name}:${entry.version}`} className="min-w-0">
      <CardHeader className="space-y-2">
        <div className="flex min-w-0 items-start justify-between gap-3">
          <CardTitle className="min-w-0 truncate text-base">{entry.title || entry.name}</CardTitle>
          <Badge variant="outline" className="shrink-0">
            {entry.source === "curated"
              ? t("settings:mcpSourceCurated")
              : t("settings:mcpSourceRegistry")}
          </Badge>
        </div>
        <p className="text-xs text-muted-foreground">
          {entry.description || t("settings:mcpNoDescription")}
        </p>
      </CardHeader>
      <CardContent className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 flex-wrap gap-2 text-xs text-muted-foreground">
          <span>{statusLabel(entry.status, t)}</span>
          <span>{t("settings:mcpChoiceCount", { count: entry.choices.length })}</span>
        </div>
        <Button
          type="button"
          size="sm"
          className={controlSizingClassName("standard", "shrink-0 cursor-pointer")}
          onClick={onReview}
        >
          {t("settings:reviewMcpEntry")}
        </Button>
      </CardContent>
    </Card>
  );
}

function MarketplaceChoiceList({
  choices,
  choice,
  onChoiceChange,
}: {
  choices: MCPMarketplaceChoice[];
  choice: MCPMarketplaceChoice | null;
  onChoiceChange: (choice: MCPMarketplaceChoice) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">{t("settings:mcpSetupChoice")}</p>
      {choices.map((item) => (
        <button
          key={item.id}
          type="button"
          className={`flex min-h-[44px] w-full flex-col items-start justify-between gap-2 sm:flex-row sm:gap-3 rounded-md border p-3 text-left cursor-pointer ${choice?.id === item.id ? "border-primary bg-primary/5" : "border-border"}`}
          onClick={() => item.selectable && onChoiceChange(item)}
          disabled={!item.selectable}
        >
          <span className="min-w-0 w-full sm:w-auto sm:flex-1">
            <span className="block truncate text-sm font-medium">{choiceLabel(item, t)}</span>
            <span className="text-xs text-muted-foreground">{item.transport || item.kind}</span>
          </span>
          <span className="min-w-0 break-words text-xs sm:max-w-[50%]">
            {choiceAvailability(item, t)}
          </span>
        </button>
      ))}
    </div>
  );
}

type MarketplaceReviewDialogProps = {
  workspaceId: string;
  inputValues: Record<string, string>;
  bindings: MCPSecretBinding[];
  onInputValuesChange: (values: Record<string, string>) => void;
  onBindingsChange: (bindings: MCPSecretBinding[]) => void;
  selected: MCPMarketplaceEntry | null;
  choice: MCPMarketplaceChoice | null;
  runtimeName: string;
  installing: boolean;
  installError: string | null;
  onClose: () => void;
  onChoiceChange: (choice: MCPMarketplaceChoice) => void;
  onRuntimeNameChange: (name: string) => void;
  onInstall: () => void;
};

function MarketplaceReviewDialog({
  workspaceId,
  inputValues,
  bindings,
  onInputValuesChange,
  onBindingsChange,
  selected,
  choice,
  runtimeName,
  installing,
  installError,
  onClose,
  onChoiceChange,
  onRuntimeNameChange,
  onInstall,
}: MarketplaceReviewDialogProps) {
  const { t } = useTranslation();
  return (
    <Dialog open={Boolean(selected)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90dvh] grid-cols-1 overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{selected?.title || selected?.name}</DialogTitle>
          <DialogDescription>{selected?.trust_notice}</DialogDescription>
        </DialogHeader>
        {selected && (
          <div className="space-y-4">
            <p className="text-sm text-muted-foreground">
              {selected.description || t("settings:mcpNoDescription")}
            </p>
            <MarketplaceChoiceList
              choices={selected.choices}
              choice={choice}
              onChoiceChange={onChoiceChange}
            />
            {choice?.selectable && (
              <>
                <MCPMarketplaceInputs
                  workspaceId={workspaceId}
                  choice={choice}
                  values={inputValues}
                  bindings={bindings}
                  onValuesChange={onInputValuesChange}
                  onBindingsChange={onBindingsChange}
                />
                {!marketplaceInputsComplete(choice, inputValues, bindings) && (
                  <p className="text-sm text-muted-foreground" role="status">
                    {t("settings:mcpRequiredInputsMissing")}
                  </p>
                )}
              </>
            )}
            <div className="space-y-2">
              <label htmlFor="mcp-marketplace-runtime" className="text-sm font-medium">
                {t("settings:mcpRuntimeName")}
              </label>
              <Input
                id="mcp-marketplace-runtime"
                value={runtimeName}
                onChange={(event) => onRuntimeNameChange(event.target.value)}
              />
            </div>
            {installError && (
              <p className="text-sm text-destructive" role="alert">
                {installError}
              </p>
            )}
          </div>
        )}
        <DialogFooter>
          <Button
            type="button"
            className={controlSizingClassName("standard", "cursor-pointer")}
            onClick={onInstall}
            disabled={
              !choice?.selectable ||
              installing ||
              !marketplaceInputsComplete(choice, inputValues, bindings)
            }
          >
            {installing ? t("settings:mcpInstalling") : t("settings:addMcpServer")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function MarketplaceResults({
  entries,
  loading,
  onReview,
}: {
  entries: MCPMarketplaceEntry[];
  loading: boolean;
  onReview: (entry: MCPMarketplaceEntry) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {entries.length === 0 && !loading && (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            {t("settings:mcpMarketplaceEmpty")}
          </CardContent>
        </Card>
      )}
      <div className="grid min-w-0 grid-cols-1 gap-3 lg:grid-cols-2">
        {entries.map((entry) => (
          <MarketplaceEntryCard
            key={`${entry.source}:${entry.name}:${entry.version}`}
            entry={entry}
            onReview={() => onReview(entry)}
          />
        ))}
      </div>
    </>
  );
}

export function MCPMarketplace({
  workspaceId,
  entries,
  stale,
  degraded,
  loading,
  onSearch,
  onRefresh,
  onInstall,
}: MCPMarketplaceProps) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<MCPMarketplaceEntry | null>(null);
  const [choice, setChoice] = useState<MCPMarketplaceChoice | null>(null);
  const [runtimeName, setRuntimeName] = useState("");
  const installingRef = useRef(false);
  const [installing, setInstalling] = useState(false);
  const [installError, setInstallError] = useState<string | null>(null);
  const [inputValues, setInputValues] = useState<Record<string, string>>({});
  const [bindings, setBindings] = useState<MCPSecretBinding[]>([]);

  const openEntry = (entry: MCPMarketplaceEntry) => {
    setInputValues({});
    setBindings([]);
    setSelected(entry);
    setChoice(entry.choices.find((item) => item.selectable) ?? entry.choices[0] ?? null);
    setRuntimeName(entry.name.split("/").pop()?.replace(/@.*$/, "") ?? entry.name);
    setInstallError(null);
  };

  const install = async () => {
    if (
      installingRef.current ||
      !selected ||
      !choice?.selectable ||
      !marketplaceInputsComplete(choice, inputValues, bindings)
    )
      return;
    installingRef.current = true;
    setInstalling(true);
    setInstallError(null);
    try {
      await onInstall({
        identity: `${selected.name}${selected.version ? `@${selected.version}` : ""}`,
        expected_revision: selected.revision,
        choice_id: choice.id,
        runtime_name: runtimeName.trim(),
        display_name: selected.title || selected.name,
        input_values: inputValues,
        secret_bindings: bindings,
      });
      setSelected(null);
    } catch (cause) {
      setInstallError(cause instanceof Error ? cause.message : t("settings:mcpInstallFailed"));
    } finally {
      installingRef.current = false;
      setInstalling(false);
    }
  };

  return (
    <div className="space-y-4" data-testid="mcp-marketplace">
      <MarketplaceSearchControls
        query={query}
        loading={loading}
        onQueryChange={setQuery}
        onSearch={() => void onSearch(query)}
        onRefresh={() => void onRefresh()}
      />
      {(stale || degraded) && (
        <p className="rounded-md border border-warning/40 bg-warning/10 p-3 text-sm" role="status">
          {degraded ? t("settings:mcpMarketplaceDegraded") : t("settings:mcpMarketplaceStale")}
        </p>
      )}
      <MarketplaceResults entries={entries} loading={loading} onReview={openEntry} />
      <MarketplaceReviewDialog
        workspaceId={workspaceId}
        inputValues={inputValues}
        bindings={bindings}
        onInputValuesChange={setInputValues}
        onBindingsChange={setBindings}
        selected={selected}
        choice={choice}
        runtimeName={runtimeName}
        installing={installing}
        installError={installError}
        onClose={() => setSelected(null)}
        onChoiceChange={(next) => {
          setChoice(next);
          setInputValues({});
          setBindings([]);
        }}
        onRuntimeNameChange={setRuntimeName}
        onInstall={() => void install()}
      />
    </div>
  );
}

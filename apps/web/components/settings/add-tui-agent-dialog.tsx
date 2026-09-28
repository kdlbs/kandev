"use client";

import { useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { t as translate } from "@/lib/i18n";
import { isHandledApiError } from "@/lib/api/client";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { MCPStrategySelect, useMCPStrategies } from "./mcp-strategy-select";

type TUIAgentFormData = {
  display_name: string;
  model?: string;
  command: string;
  mcp_strategy?: string;
  protocol?: string;
};

// The placeholder kandev substitutes into the TUI command. The user types it
// verbatim, so it is interpolated as a value rather than written into the
// catalog. Same for the example command and model names.
const MODEL_TOKEN = "{{model}}";

// Radix Select cannot hold an empty-string value, and the stored protocol for
// terminal passthrough is exactly that, so the terminal choice needs a
// sentinel. It is a control value, never displayed and never translated.
const PROTOCOL_TERMINAL = "terminal";
const PROTOCOL_ACP = "acp";

// i18n-exempt: example shell command, typed verbatim rather than translated.
const ACP_COMMAND_EXAMPLE = "superagent --acp";

type AddTUIAgentDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (data: TUIAgentFormData) => Promise<void>;
};

type DialogHandlersParams = {
  displayName: string;
  model: string;
  command: string;
  mcpStrategy: string;
  protocol: string;
  setError: React.Dispatch<React.SetStateAction<string | null>>;
  setLoading: React.Dispatch<React.SetStateAction<boolean>>;
  onSubmit: (data: TUIAgentFormData) => Promise<void>;
  onOpenChange: (open: boolean) => void;
  reset: () => void;
};

function useDialogHandlers({
  displayName,
  model,
  command,
  mcpStrategy,
  protocol,
  setError,
  setLoading,
  onSubmit,
  onOpenChange,
  reset,
}: DialogHandlersParams) {
  const handleSubmit = async () => {
    if (!displayName.trim()) {
      setError(translate("agents:displayNameRequired"));
      return;
    }
    if (!command.trim()) {
      setError(translate("agents:commandRequired"));
      return;
    }
    setError(null);
    setLoading(true);
    const acp = protocol === PROTOCOL_ACP;
    try {
      await onSubmit({
        display_name: displayName.trim(),
        // An ACP profile's model comes from the capability probe, and its MCP
        // servers travel in session/new, so neither field is offered — sending
        // one anyway would persist a value nothing reads.
        model: acp ? undefined : model.trim() || undefined,
        command: command.trim(),
        mcp_strategy: acp ? undefined : mcpStrategy || undefined,
        protocol: acp ? PROTOCOL_ACP : undefined,
      });
      reset();
      onOpenChange(false);
    } catch (err) {
      if (isHandledApiError(err)) {
        reset();
        onOpenChange(false);
        return;
      }
      setError(err instanceof Error ? err.message : translate("agents:failedToCreateAgent"));
    } finally {
      setLoading(false);
    }
  };

  const handleOpenChange = (next: boolean) => {
    if (!next) reset();
    onOpenChange(next);
  };

  return { handleSubmit, handleOpenChange };
}

/**
 * Picks the runtime kandev drives the command with.
 *
 * This is an explicit choice rather than something probed from the command.
 * A CLI that serves ACP normally does so behind a flag (`--acp`, `acp`), and
 * the command that starts the server is not the command that renders an
 * interactive terminal — so there is nothing to detect from a single command
 * string, and guessing wrong launches a JSON-RPC server into a terminal tab.
 */
function AgentProtocolSelect({
  value,
  onChange,
}: {
  value: string;
  onChange: (next: string) => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="space-y-2">
      <Label htmlFor="tui-protocol">{t("agents:agentProtocol")}</Label>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger
          id="tui-protocol"
          className="w-full min-w-0 cursor-pointer [@media(pointer:coarse)]:min-h-11"
          data-testid="agent-protocol-select"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem
            value={PROTOCOL_TERMINAL}
            description={t("agents:agentProtocolTerminalHelp")}
            className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
          >
            {t("agents:agentProtocolTerminal")}
          </SelectItem>
          <SelectItem
            value={PROTOCOL_ACP}
            description={t("agents:agentProtocolAcpHelp")}
            className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
          >
            {t("agents:agentProtocolAcp")}
          </SelectItem>
        </SelectContent>
      </Select>
    </div>
  );
}

/** The model label and the command hint only make sense in terminal mode: an
 * ACP profile's model comes from the capability probe, not from the command. */
function TerminalModelField({
  value,
  onChange,
}: {
  value: string;
  onChange: (next: string) => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="space-y-2">
      <Label htmlFor="tui-model">{t("agents:model")}</Label>
      <Input
        id="tui-model"
        placeholder={t("agents:exampleValue", { example: "best" })}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      <p className="text-xs text-muted-foreground">{t("agents:tuiModelHelp")}</p>
    </div>
  );
}

function CommandField({
  acp,
  value,
  onChange,
}: {
  acp: boolean;
  value: string;
  onChange: (next: string) => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="space-y-2">
      <Label htmlFor="tui-command">{t("agents:command")}</Label>
      <Input
        id="tui-command"
        placeholder={t("agents:exampleValue", {
          example: acp ? ACP_COMMAND_EXAMPLE : `superagent --yolo --model ${MODEL_TOKEN}`,
        })}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      {acp ? (
        <p className="text-xs text-muted-foreground">
          {t("agents:agentProtocolHelp", { example: ACP_COMMAND_EXAMPLE })}
        </p>
      ) : (
        <p className="text-xs text-muted-foreground">
          <Trans i18nKey="agents:tuiCommandHelp" values={{ token: MODEL_TOKEN }}>
            <code className="rounded bg-muted px-1 py-0.5" />
          </Trans>
        </p>
      )}
    </div>
  );
}

export function AddTUIAgentDialog({ open, onOpenChange, onSubmit }: AddTUIAgentDialogProps) {
  const { t } = useTranslation();
  const [displayName, setDisplayName] = useState("");
  const [model, setModel] = useState("");
  const [command, setCommand] = useState("");
  const [mcpStrategy, setMcpStrategy] = useState("");
  const [protocol, setProtocol] = useState(PROTOCOL_TERMINAL);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const acp = protocol === PROTOCOL_ACP;

  // Only fetch the strategy list while the dialog is open on the protocol that
  // can use one.
  const strategies = useMCPStrategies(open && !acp);

  const reset = () => {
    setDisplayName("");
    setModel("");
    setCommand("");
    setMcpStrategy("");
    setProtocol(PROTOCOL_TERMINAL);
    setError(null);
    setLoading(false);
  };

  const { handleSubmit, handleOpenChange } = useDialogHandlers({
    displayName,
    model,
    command,
    mcpStrategy,
    protocol,
    setError,
    setLoading,
    onSubmit,
    onOpenChange,
    reset,
  });

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("agents:addTuiAgent")}</DialogTitle>
          <DialogDescription>{t("agents:addTuiAgentDescription")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="tui-display-name">{t("agents:displayName")}</Label>
            <Input
              id="tui-display-name"
              placeholder={t("agents:exampleValue", { example: "superagent" })}
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
            />
          </div>
          <AgentProtocolSelect value={protocol} onChange={setProtocol} />
          {!acp && <TerminalModelField value={model} onChange={setModel} />}
          <CommandField acp={acp} value={command} onChange={setCommand} />
          {!acp && (
            <MCPStrategySelect
              id="tui-mcp-strategy"
              value={mcpStrategy}
              onChange={setMcpStrategy}
              strategies={strategies}
            />
          )}
          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            {t("common:cancel")}
          </Button>
          <Button onClick={handleSubmit} disabled={loading} className="cursor-pointer">
            {loading ? t("agents:creating") : t("agents:create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

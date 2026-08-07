"use client";

import { useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { t as translate } from "@/lib/i18n";
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

type TUIAgentFormData = { display_name: string; model?: string; command: string };

// The placeholder kandev substitutes into the TUI command. The user types it
// verbatim, so it is interpolated as a value rather than written into the
// catalog. Same for the example command and model names.
const MODEL_TOKEN = "{{model}}";

type AddTUIAgentDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (data: TUIAgentFormData) => Promise<void>;
};

type DialogHandlersParams = {
  displayName: string;
  model: string;
  command: string;
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
    try {
      await onSubmit({
        display_name: displayName.trim(),
        model: model.trim() || undefined,
        command: command.trim(),
      });
      reset();
      onOpenChange(false);
    } catch (err) {
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

export function AddTUIAgentDialog({ open, onOpenChange, onSubmit }: AddTUIAgentDialogProps) {
  const { t } = useTranslation();
  const [displayName, setDisplayName] = useState("");
  const [model, setModel] = useState("");
  const [command, setCommand] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reset = () => {
    setDisplayName("");
    setModel("");
    setCommand("");
    setError(null);
    setLoading(false);
  };

  const { handleSubmit, handleOpenChange } = useDialogHandlers({
    displayName,
    model,
    command,
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
              placeholder={t("agents:exampleValue", { example: "superclaude" })}
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="tui-model">{t("agents:model")}</Label>
            <Input
              id="tui-model"
              placeholder={t("agents:exampleValue", { example: "best" })}
              value={model}
              onChange={(e) => setModel(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">{t("agents:tuiModelHelp")}</p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="tui-command">{t("agents:command")}</Label>
            <Input
              id="tui-command"
              placeholder={t("agents:exampleValue", {
                example: `superclaude --yolo --model ${MODEL_TOKEN}`,
              })}
              value={command}
              onChange={(e) => setCommand(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              <Trans i18nKey="agents:tuiCommandHelp" values={{ token: MODEL_TOKEN }}>
                <code className="rounded bg-muted px-1 py-0.5" />
              </Trans>
            </p>
          </div>
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

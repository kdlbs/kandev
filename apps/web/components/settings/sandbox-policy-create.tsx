"use client";

import { settingsActionClassName } from "./settings-control";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { publishSandboxPolicy } from "@/lib/api/domains/sandbox-api";
import { buildSandboxPolicy, emptySandboxPolicyDraft } from "@/lib/sandbox-policy-draft";
import { sandboxErrorMessageKey } from "@/lib/sandbox-messages";
import { generateUUID } from "@/lib/uuid";
import type { SandboxProvider, SandboxRevision } from "@/lib/types/sandbox";
import { SandboxPolicyFields } from "./sandbox-policy-fields";
import { SandboxPermissions } from "./sandbox-permissions";

export function SandboxPolicyCreate({
  provider,
  onCreated,
}: {
  provider: SandboxProvider | undefined;
  onCreated: (revision: SandboxRevision) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        type="button"
        variant="outline"
        className={settingsActionClassName()}
        disabled={!provider}
        onClick={() => setOpen(true)}
      >
        {t("agents:sandboxCreatePolicy")}
      </Button>
      {provider && open && (
        <PolicyCreateDialog
          provider={provider}
          onClose={() => setOpen(false)}
          onCreated={onCreated}
        />
      )}
    </>
  );
}

function usePolicyPublication(
  provider: SandboxProvider,
  onCreated: (revision: SandboxRevision) => void,
  onClose: () => void,
) {
  const [id] = useState(generateUUID);
  const [draft, setDraft] = useState(emptySandboxPolicyDraft);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<ReturnType<typeof sandboxErrorMessageKey> | null>(null);
  const busy = useRef(false);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const policy = buildSandboxPolicy(id, provider, draft);
  const publish = async () => {
    if (!policy || busy.current) return;
    busy.current = true;
    setSaving(true);
    setError(null);
    try {
      const revision = await publishSandboxPolicy(policy, 0);
      if (!mounted.current) return;
      onCreated(revision);
      onClose();
    } catch (reason) {
      if (mounted.current) setError(sandboxErrorMessageKey(reason));
    } finally {
      busy.current = false;
      if (mounted.current) setSaving(false);
    }
  };
  return { draft, setDraft, saving, error, policy, publish };
}

function PolicyCreateDialog({
  provider,
  onCreated,
  onClose,
}: {
  provider: SandboxProvider;
  onCreated: (revision: SandboxRevision) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const { usesDesktopWorkbench } = useResponsiveBreakpoint();
  const form = usePolicyPublication(provider, onCreated, onClose);
  const layout = usesDesktopWorkbench
    ? "max-h-[calc(100dvh-2rem)] max-w-2xl"
    : "inset-0 h-dvh w-full max-w-none translate-x-0 translate-y-0 rounded-none border-0 pt-[max(1rem,env(safe-area-inset-top))]";
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !form.saving) onClose();
      }}
    >
      <DialogContent
        showCloseButton={false}
        enterConfirms={false}
        onInteractOutside={(event) => event.preventDefault()}
        className={`flex flex-col overflow-hidden pb-[max(1rem,env(safe-area-inset-bottom))] ${layout}`}
      >
        <DialogHeader className="shrink-0 text-left">
          <DialogTitle>{t("agents:sandboxCreatePolicy")}</DialogTitle>
          <DialogDescription>
            {t("agents:sandboxCreatePolicyHelp", { provider: provider.display_name })}
          </DialogDescription>
        </DialogHeader>
        <div
          className="min-h-0 flex-1 space-y-5 overflow-y-auto overscroll-contain px-1 py-2"
          data-testid="sandbox-policy-create-form"
        >
          <fieldset disabled={form.saving}>
            <SandboxPolicyFields
              draft={form.draft}
              update={(patch) => form.setDraft((current) => ({ ...current, ...patch }))}
            />
          </fieldset>
          {form.policy && <SandboxPermissions grants={form.policy.grants} />}
        </div>
        {form.error && (
          <p role="alert" className="text-sm text-destructive">
            {t(form.error)}
          </p>
        )}
        <div className="flex shrink-0 items-center justify-between gap-3">
          <Button
            type="button"
            variant="outline"
            className={settingsActionClassName()}
            disabled={form.saving}
            onClick={onClose}
          >
            {t("common:cancel")}
          </Button>
          <Button
            type="button"
            className={settingsActionClassName()}
            disabled={!form.policy || form.saving}
            onClick={() => void form.publish()}
          >
            {t("agents:sandboxCreatePolicy")}
          </Button>
        </div>
        {form.saving && (
          <p role="status" className="text-sm">
            {t("common:loading")}
          </p>
        )}
      </DialogContent>
    </Dialog>
  );
}

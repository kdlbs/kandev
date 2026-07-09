"use client";

import { useCallback, useState } from "react";
import { IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import {
  createSentryInstance,
  SENTRY_ERROR_CODES,
  sentryErrorCode,
  testSentryConnection,
  testSentryInstance,
  updateSentryInstance,
} from "@/lib/api/domains/sentry-api";
import {
  SENTRY_AUTH_METHOD,
  SENTRY_DEFAULT_URL,
  type SentryConfig,
  type TestSentryConnectionResult,
} from "@/lib/types/sentry";

const FIELD = "space-y-1.5";
const HELP = "text-xs text-muted-foreground";

type FormState = { name: string; url: string; secret: string };

function instanceToForm(instance: SentryConfig | null): FormState {
  return { name: instance?.name ?? "", url: instance?.url || SENTRY_DEFAULT_URL, secret: "" };
}

type FieldProps = {
  form: FormState;
  idPrefix: string;
  loading: boolean;
  update: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
};

function NameField({ form, idPrefix, loading, update }: FieldProps) {
  return (
    <div className={FIELD}>
      <Label htmlFor={`${idPrefix}-name`}>Name</Label>
      <Input
        id={`${idPrefix}-name`}
        data-testid={`${idPrefix}-name-input`}
        placeholder="Production, Self-hosted, …"
        value={form.name}
        onChange={(e) => update("name", e.target.value)}
        disabled={loading}
      />
      <p className={HELP}>A label for this instance. Must be unique within the workspace.</p>
    </div>
  );
}

function UrlField({ form, idPrefix, loading, update }: FieldProps) {
  return (
    <div className={FIELD}>
      <Label htmlFor={`${idPrefix}-url`}>Instance URL</Label>
      <Input
        id={`${idPrefix}-url`}
        data-testid={`${idPrefix}-url-input`}
        type="url"
        placeholder={SENTRY_DEFAULT_URL}
        value={form.url}
        onChange={(e) => update("url", e.target.value)}
        disabled={loading}
      />
      <p className={HELP}>
        Base URL of your Sentry instance. Leave as {SENTRY_DEFAULT_URL} for Sentry SaaS, or point it
        at a self-hosted install (e.g. https://sentry.your-company.com).
      </p>
    </div>
  );
}

function SecretField({
  form,
  idPrefix,
  loading,
  update,
  hasSavedSecret,
}: FieldProps & { hasSavedSecret: boolean }) {
  return (
    <div className={FIELD}>
      <div className="flex items-center gap-1.5">
        <Label htmlFor={`${idPrefix}-secret`}>
          Auth token
          {hasSavedSecret && (
            <span className="text-xs text-muted-foreground ml-2">
              (saved — leave blank to keep the current value)
            </span>
          )}
        </Label>
        <Tooltip>
          <TooltipTrigger asChild>
            <IconInfoCircle
              className="h-3.5 w-3.5 text-muted-foreground/50 hover:text-muted-foreground cursor-help shrink-0"
              aria-label="Required token scopes"
            />
          </TooltipTrigger>
          <TooltipContent className="max-w-xs" align="start">
            <p className="text-xs font-medium mb-1">Grant Read access to these scopes:</p>
            <ul className="text-xs space-y-0.5">
              <li>
                <code className="text-[10px] bg-white/15 px-1 rounded">org:read</code>{" "}
                <span className="opacity-70">Organization — resolve the org and list issues</span>
              </li>
              <li>
                <code className="text-[10px] bg-white/15 px-1 rounded">project:read</code>{" "}
                <span className="opacity-70">Project — list projects and scope searches</span>
              </li>
              <li>
                <code className="text-[10px] bg-white/15 px-1 rounded">event:read</code>{" "}
                <span className="opacity-70">
                  Issue &amp; Event — browse issues and run watchers
                </span>
              </li>
            </ul>
          </TooltipContent>
        </Tooltip>
      </div>
      <Input
        id={`${idPrefix}-secret`}
        data-testid={`${idPrefix}-secret-input`}
        type="password"
        placeholder={hasSavedSecret ? "••••••••" : "sntrys_..."}
        value={form.secret}
        onChange={(e) => update("secret", e.target.value)}
        disabled={loading}
      />
    </div>
  );
}

function TestResultAlert({ result }: { result: TestSentryConnectionResult | null }) {
  if (!result) return null;
  return (
    <Alert variant={result.ok ? "default" : "destructive"}>
      <AlertDescription>
        {result.ok
          ? `Connected as ${result.displayName || result.email || result.userId}`
          : `Failed: ${result.error}`}
      </AlertDescription>
    </Alert>
  );
}

type UseInstanceFormArgs = {
  workspaceId: string;
  instance: SentryConfig | null;
  form: FormState;
  onSaved: (cfg: SentryConfig) => void;
};

function useInstanceForm({ workspaceId, instance, form, onSaved }: UseInstanceFormArgs) {
  const { toast } = useToast();
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestSentryConnectionResult | null>(null);

  const handleTest = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      // A typed secret tests the new credentials; a saved instance with a blank
      // secret re-tests its stored token.
      const res =
        form.secret || !instance
          ? await testSentryConnection(workspaceId, {
              secret: form.secret || undefined,
              url: form.url || undefined,
              authMethod: SENTRY_AUTH_METHOD,
            })
          : await testSentryInstance(workspaceId, instance.id);
      setTestResult(res);
    } catch (err) {
      setTestResult({ ok: false, error: String(err) });
    } finally {
      setTesting(false);
    }
  }, [workspaceId, instance, form.secret, form.url]);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const saved = instance
        ? await updateSentryInstance(workspaceId, instance.id, {
            name: form.name.trim(),
            authMethod: SENTRY_AUTH_METHOD,
            url: form.url,
            secret: form.secret,
          })
        : await createSentryInstance(workspaceId, {
            workspaceId,
            name: form.name.trim(),
            authMethod: SENTRY_AUTH_METHOD,
            url: form.url,
            secret: form.secret,
          });
      toast({ description: "Sentry instance saved", variant: "success" });
      onSaved(saved);
    } catch (err) {
      const message =
        sentryErrorCode(err) === SENTRY_ERROR_CODES.nameTaken
          ? `An instance named "${form.name.trim()}" already exists in this workspace.`
          : `Save failed: ${String(err)}`;
      toast({ description: message, variant: "error" });
    } finally {
      setSaving(false);
    }
  }, [workspaceId, instance, form, toast, onSaved]);

  return { saving, testing, testResult, handleTest, handleSave };
}

type SentryInstanceFormProps = {
  workspaceId: string;
  // instance === null creates a new instance; otherwise the form edits it.
  instance: SentryConfig | null;
  // idPrefix scopes element ids + testids so the mutually-exclusive add/edit
  // forms never collide (e.g. "sentry-add" vs "sentry-edit").
  idPrefix: string;
  onSaved: (cfg: SentryConfig) => void;
  onCancel: () => void;
};

export function SentryInstanceForm({
  workspaceId,
  instance,
  idPrefix,
  onSaved,
  onCancel,
}: SentryInstanceFormProps) {
  const [form, setForm] = useState<FormState>(() => instanceToForm(instance));
  const update = useCallback(
    <K extends keyof FormState>(key: K, value: FormState[K]) =>
      setForm((prev) => ({ ...prev, [key]: value })),
    [],
  );
  const { saving, testing, testResult, handleTest, handleSave } = useInstanceForm({
    workspaceId,
    instance,
    form,
    onSaved,
  });

  const hasSavedSecret = !!instance?.hasSecret;
  const missingSecret = !hasSavedSecret && !form.secret;
  const disableSave = saving || !form.name.trim() || missingSecret;
  const disableTest = testing || missingSecret;
  let saveLabel = instance ? "Update" : "Save";
  if (saving) saveLabel = "Saving...";

  return (
    <div className="space-y-4 rounded-md border p-4" data-testid={`${idPrefix}-form`}>
      <NameField form={form} idPrefix={idPrefix} loading={saving} update={update} />
      <UrlField form={form} idPrefix={idPrefix} loading={saving} update={update} />
      <SecretField
        form={form}
        idPrefix={idPrefix}
        loading={saving}
        update={update}
        hasSavedSecret={hasSavedSecret}
      />
      <TestResultAlert result={testResult} />
      <Separator />
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          onClick={handleTest}
          disabled={disableTest}
          className="cursor-pointer"
          title={missingSecret ? "Paste an auth token to test the connection" : undefined}
          data-testid={`${idPrefix}-test-button`}
        >
          {testing ? "Testing..." : "Test connection"}
        </Button>
        <Button
          type="button"
          onClick={handleSave}
          disabled={disableSave}
          className="cursor-pointer"
          data-testid={`${idPrefix}-save-button`}
        >
          {saveLabel}
        </Button>
        <Button
          type="button"
          variant="ghost"
          onClick={onCancel}
          className="ml-auto cursor-pointer"
          data-testid={`${idPrefix}-cancel-button`}
        >
          Cancel
        </Button>
      </div>
    </div>
  );
}

"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
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
import { useTranslation } from "react-i18next";

const FIELD = "space-y-1.5";
const HELP = "text-xs text-muted-foreground";

// None of the four below is copy, so none reaches the catalog — the
// pseudo-locale would transliterate them into values the user cannot act on.
// SECRET_MASK is a glyph run standing in for a stored token; AUTH_TOKEN_HINT is
// the literal prefix every Sentry auth token carries; SELF_HOSTED_EXAMPLE_URL is
// a hostname; and each TOKEN_SCOPES `scope` is an API scope identifier the user
// must tick verbatim in Sentry. Only the scope descriptions are translatable, so
// they travel as `descriptionKey` and resolve at render.
const SECRET_MASK = "••••••••";
const AUTH_TOKEN_HINT = "sntrys_...";
const SELF_HOSTED_EXAMPLE_URL = "https://sentry.your-company.com";
const TOKEN_SCOPES: { scope: string; descriptionKey: string }[] = [
  { scope: "org:read", descriptionKey: "sentry:scopeOrgRead" },
  { scope: "project:read", descriptionKey: "sentry:scopeProjectRead" },
  { scope: "event:read", descriptionKey: "sentry:scopeEventRead" },
];

type FormState = { name: string; url: string; secret: string };

function instanceToForm(instance: SentryConfig | null): FormState {
  return { name: instance?.name ?? "", url: instance?.url || SENTRY_DEFAULT_URL, secret: "" };
}

type FieldProps = {
  form: FormState;
  baseline: FormState;
  idPrefix: string;
  loading: boolean;
  update: <K extends keyof FormState>(key: K, value: FormState[K]) => void;
};

function NameField({ form, baseline, idPrefix, loading, update }: FieldProps) {
  const { t } = useTranslation();
  return (
    <div className={FIELD}>
      <Label htmlFor={`${idPrefix}-name`}>{t("sentry:name")}</Label>
      <Input
        id={`${idPrefix}-name`}
        data-testid={`${idPrefix}-name-input`}
        placeholder={t("sentry:namePlaceholder")}
        value={form.name}
        data-settings-dirty={form.name !== baseline.name}
        onChange={(e) => update("name", e.target.value)}
        disabled={loading}
      />
      <p className={HELP}>{t("sentry:nameHelp")}</p>
    </div>
  );
}

function UrlField({ form, baseline, idPrefix, loading, update }: FieldProps) {
  const { t } = useTranslation();
  return (
    <div className={FIELD}>
      <Label htmlFor={`${idPrefix}-url`}>{t("sentry:instanceUrl")}</Label>
      <Input
        id={`${idPrefix}-url`}
        data-testid={`${idPrefix}-url-input`}
        type="url"
        placeholder={SENTRY_DEFAULT_URL}
        value={form.url}
        data-settings-dirty={form.url !== baseline.url}
        onChange={(e) => update("url", e.target.value)}
        disabled={loading}
      />
      {/* Both URLs are interpolated rather than written into the message: the
          pseudo-locale must not transliterate a hostname the user has to type. */}
      <p className={HELP}>
        {t("sentry:instanceUrlHelp", {
          defaultUrl: SENTRY_DEFAULT_URL,
          exampleUrl: SELF_HOSTED_EXAMPLE_URL,
        })}
      </p>
    </div>
  );
}

function SecretField({
  form,
  baseline,
  idPrefix,
  loading,
  update,
  hasSavedSecret,
}: FieldProps & { hasSavedSecret: boolean }) {
  const { t } = useTranslation();
  return (
    <div className={FIELD}>
      <div className="flex items-center gap-1.5">
        <Label htmlFor={`${idPrefix}-secret`}>
          {t("sentry:authToken")}
          {hasSavedSecret && (
            <span className="text-xs text-muted-foreground ml-2">
              {t("sentry:savedLeaveBlank")}
            </span>
          )}
        </Label>
        <Tooltip>
          <TooltipTrigger asChild>
            <IconInfoCircle
              className="h-3.5 w-3.5 text-muted-foreground/50 hover:text-muted-foreground cursor-help shrink-0"
              aria-label={t("sentry:requiredTokenScopes")}
            />
          </TooltipTrigger>
          <TooltipContent className="max-w-xs" align="start">
            <p className="text-xs font-medium mb-1">{t("sentry:grantReadAccess")}</p>
            <ul className="text-xs space-y-0.5">
              {TOKEN_SCOPES.map(({ scope, descriptionKey }) => (
                <li key={scope}>
                  <code className="text-[10px] bg-white/15 px-1 rounded">{scope}</code>{" "}
                  <span className="opacity-70">{t(descriptionKey)}</span>
                </li>
              ))}
            </ul>
          </TooltipContent>
        </Tooltip>
      </div>
      <Input
        id={`${idPrefix}-secret`}
        data-testid={`${idPrefix}-secret-input`}
        type="password"
        placeholder={hasSavedSecret ? SECRET_MASK : AUTH_TOKEN_HINT}
        value={form.secret}
        data-settings-dirty={form.secret !== baseline.secret}
        onChange={(e) => update("secret", e.target.value)}
        disabled={loading}
      />
    </div>
  );
}

function TestResultAlert({ result }: { result: TestSentryConnectionResult | null }) {
  const { t } = useTranslation();
  if (!result) return null;
  return (
    <Alert variant={result.ok ? "default" : "destructive"}>
      {/* The identity and the error text are both server data, interpolated so
          neither is ever routed through the catalog. */}
      <AlertDescription>
        {result.ok
          ? t("sentry:connectedAs", {
              name: result.displayName || result.email || result.userId,
            })
          : t("sentry:testFailed", { error: result.error })}
      </AlertDescription>
    </Alert>
  );
}

type UseInstanceFormArgs = {
  workspaceId: string;
  instance: SentryConfig | null;
  form: FormState;
};

function useInstanceForm({ workspaceId, instance, form }: UseInstanceFormArgs) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestSentryConnectionResult | null>(null);

  // A saved instance can re-test its stored token only while its URL is
  // unchanged. Any typed token or URL change is a candidate configuration.
  const candidateTest = !instance || !!form.secret || form.url !== instance.url;
  const handleTest = useCallback(async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const res = candidateTest
        ? await testSentryConnection(workspaceId, {
            secret: form.secret || undefined,
            url: form.url || undefined,
            authMethod: SENTRY_AUTH_METHOD,
          })
        : await testSentryInstance(workspaceId, instance!.id);
      setTestResult(res);
    } catch (err) {
      setTestResult({ ok: false, error: String(err) });
    } finally {
      setTesting(false);
    }
  }, [workspaceId, instance, candidateTest, form.secret, form.url]);

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
      toast({ description: t("sentry:instanceSaved"), variant: "success" });
      return saved;
    } catch (err) {
      // The name is the user's own; the fallback carries the raw server error.
      // Both are interpolated so neither is translated.
      const message =
        sentryErrorCode(err) === SENTRY_ERROR_CODES.nameTaken
          ? t("sentry:instanceNameTaken", { name: form.name.trim() })
          : t("sentry:saveFailed", { error: String(err) });
      toast({ description: message, variant: "error" });
      throw err;
    } finally {
      setSaving(false);
    }
  }, [workspaceId, instance, form, toast, t]);

  return { saving, testing, testResult, candidateTest, handleTest, handleSave };
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
  onDirtyChange?: (isDirty: boolean) => void;
};

type CoordinatedSaveOptions = {
  instance: SentryConfig | null;
  form: FormState;
  setForm: (form: FormState) => void;
  handleSave: () => Promise<SentryConfig>;
  onSaved: (cfg: SentryConfig) => void;
  canSave: boolean;
};

function useCoordinatedInstanceSave({
  instance,
  form,
  setForm,
  handleSave,
  onSaved,
  canSave,
}: CoordinatedSaveOptions) {
  const { t } = useTranslation();
  const [baseline, setBaseline] = useState<FormState>(() => instanceToForm(instance));
  const revision = JSON.stringify(form);
  const latestRevision = useRef(revision);
  latestRevision.current = revision;
  const isDirty = instance === null || revision !== JSON.stringify(baseline);

  useSettingsSaveContributor({
    id: `sentry-instance:${instance?.id ?? "new"}`,
    revision,
    isDirty,
    canSave,
    invalidReason: canSave ? undefined : t("sentry:enterNameAndTokenBeforeSaving"),
    save: async (submittedRevision) => {
      const submitted = form;
      const saved = await handleSave();
      setBaseline(submitted);
      if (instance === null || latestRevision.current === submittedRevision) onSaved(saved);
    },
    discard: () => setForm(baseline),
  });

  return { baseline, isDirty };
}

type FormActionsProps = {
  idPrefix: string;
  testing: boolean;
  disableTest: boolean;
  requiresTestSecret: boolean;
  onTest: () => void;
  onCancel: () => void;
};

function FormActions({
  idPrefix,
  testing,
  disableTest,
  requiresTestSecret,
  onTest,
  onCancel,
}: FormActionsProps) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        type="button"
        variant="outline"
        onClick={onTest}
        disabled={disableTest}
        className="cursor-pointer"
        title={requiresTestSecret ? t("sentry:pasteAnAuthTokenToTest") : undefined}
        data-testid={`${idPrefix}-test-button`}
      >
        {testing ? t("sentry:testing") : t("sentry:testConnection")}
      </Button>
      <Button
        type="button"
        variant="ghost"
        onClick={onCancel}
        className="ml-auto cursor-pointer"
        data-testid={`${idPrefix}-cancel-button`}
      >
        {t("common:cancel")}
      </Button>
    </div>
  );
}

export function SentryInstanceForm({
  workspaceId,
  instance,
  idPrefix,
  onSaved,
  onCancel,
  onDirtyChange,
}: SentryInstanceFormProps) {
  const { t } = useTranslation();
  const [form, setForm] = useState<FormState>(() => instanceToForm(instance));
  const update = useCallback(
    <K extends keyof FormState>(key: K, value: FormState[K]) =>
      setForm((prev) => ({ ...prev, [key]: value })),
    [],
  );
  const { saving, testing, testResult, candidateTest, handleTest, handleSave } = useInstanceForm({
    workspaceId,
    instance,
    form,
  });

  const hasSavedSecret = !!instance?.hasSecret;
  const missingSecret = !hasSavedSecret && !form.secret;
  const requiresTestSecret = !form.secret && (candidateTest || missingSecret);
  const disableTest = testing || requiresTestSecret;
  const canSave = Boolean(form.name.trim()) && !missingSecret;
  const coordinated = useCoordinatedInstanceSave({
    instance,
    form,
    setForm,
    handleSave,
    onSaved,
    canSave,
  });
  useEffect(() => onDirtyChange?.(coordinated.isDirty), [coordinated.isDirty, onDirtyChange]);
  useEffect(() => () => onDirtyChange?.(false), [onDirtyChange]);

  return (
    <div
      className="space-y-4 rounded-md border p-4"
      data-testid={`${idPrefix}-form`}
      data-settings-dirty={coordinated.isDirty}
      data-settings-dirty-level="container"
    >
      {instance === null && (
        <h4 data-testid={`${idPrefix}-form-heading`} className="text-sm font-semibold">
          {t("sentry:newInstance")}
        </h4>
      )}
      <NameField
        form={form}
        baseline={coordinated.baseline}
        idPrefix={idPrefix}
        loading={saving}
        update={update}
      />
      <UrlField
        form={form}
        baseline={coordinated.baseline}
        idPrefix={idPrefix}
        loading={saving}
        update={update}
      />
      <SecretField
        form={form}
        baseline={coordinated.baseline}
        idPrefix={idPrefix}
        loading={saving}
        update={update}
        hasSavedSecret={hasSavedSecret}
      />
      <TestResultAlert result={testResult} />
      <Separator />
      <FormActions
        idPrefix={idPrefix}
        testing={testing}
        disableTest={disableTest}
        requiresTestSecret={requiresTestSecret}
        onTest={handleTest}
        onCancel={onCancel}
      />
    </div>
  );
}

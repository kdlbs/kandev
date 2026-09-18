import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { useMaintenancePage } from "@/hooks/domains/orchestration/use-assistant-maintenance";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import type {
  ImprovementDetail,
  MaintenanceGrantRequest,
  MaintenanceScope,
} from "@/lib/api/domains/assistant-maintenance-types";
import { MaintenanceSelect } from "./maintenance-select";
export type MaintenanceGrantFormProps = {
  binding: AssistantBinding;
  detail: ImprovementDetail;
  busy: boolean;
  onSave: (request: MaintenanceGrantRequest) => void;
};
function useGrantDraft({ binding, detail, onSave }: MaintenanceGrantFormProps) {
  const [draft, setDraft] = useState<MaintenanceScope>(
    () =>
      detail.grant?.scope ?? {
        repository_id: "",
        workflow_id: "",
        workflow_step_id: "",
        profile_id: "",
        files: [],
        actions: ["read", "patch", "test", "commit"],
        image: "",
        positive_check: ["node", "tests/positive.js"],
        negative_check: ["node", "tests/negative.js"],
      },
  );
  const [files, setFiles] = useState(draft.files.join("\n"));
  const [positive, setPositive] = useState(JSON.stringify(draft.positive_check));
  const [negative, setNegative] = useState(JSON.stringify(draft.negative_check));
  const [hours, setHours] = useState("24");
  const [invalid, setInvalid] = useState(false);
  const set = (key: keyof MaintenanceScope, value: string) =>
    setDraft((old) => ({
      ...old,
      [key]: value,
      ...(key === "workflow_id" ? { workflow_step_id: "" } : {}),
    }));
  const submit = () => {
    const scope = grantScope(draft, files, positive, negative);
    const expiry = Number(hours);
    if (!scope || !Number.isFinite(expiry) || expiry < 1 || expiry > 168) {
      setInvalid(true);
      return;
    }
    setInvalid(false);
    onSave({
      scope,
      expires_at: new Date(Date.now() + expiry * 3_600_000).toISOString(),
      expected_binding_version: binding.version,
      expected_revision: detail.grant?.revision ?? 0,
      candidate_revision: detail.candidate.revision,
    });
  };
  return {
    draft,
    set,
    files,
    setFiles,
    positive,
    setPositive,
    negative,
    setNegative,
    hours,
    setHours,
    invalid,
    submit,
  };
}

const fields = [
  { key: "repository_id", kind: "repository" },
  { key: "workflow_id", kind: "workflow" },
  { key: "workflow_step_id", kind: "step" },
  { key: "profile_id", kind: "profile" },
] as const;
export function MaintenanceGrantForm(props: MaintenanceGrantFormProps) {
  const { binding, detail, busy } = props;
  const { t } = useTranslation();
  const options = useMaintenancePage(
    "options",
    binding,
    detail.candidate.id,
    detail.candidate.revision,
  );
  const form = useGrantDraft(props);
  const { draft, set, submit, invalid } = form;
  return (
    <form
      className="space-y-3 rounded-md border p-3 min-w-0"
      data-testid="maintenance-grant-form"
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      <p className="text-sm">{t("orchestration:maintenanceGrantHint")}</p>
      <fieldset disabled={busy} className="space-y-3 min-w-0">
        {fields.map(({ key, kind }) => (
          <MaintenanceSelect
            key={key}
            label={t(`orchestration:maintenanceField_${key}`)}
            value={draft[key]}
            onChange={(value) => set(key, value)}
            options={options.entries
              .filter(
                (row) =>
                  row.kind === kind && (kind !== "step" || row.workflow_id === draft.workflow_id),
              )
              .map((row) => ({ id: row.resource_id, name: row.name }))}
          />
        ))}
        {options.nextCursor && (
          <Button
            type="button"
            variant="outline"
            className="cursor-pointer max-md:min-h-11"
            onClick={() => void options.loadMore()}
          >
            {t("orchestration:maintenanceMoreChoices")}
          </Button>
        )}
        {Boolean(options.error) && <p role="alert">{t("orchestration:assistantUnavailable")}</p>}
        <GrantTextField
          label={t("orchestration:maintenanceFiles")}
          value={form.files}
          onChange={form.setFiles}
          testId="maintenance-files"
        />
        <label className="block text-sm space-y-1">
          <span>{t("orchestration:maintenanceImage")}</span>
          <Input
            value={draft.image}
            onChange={(event) => set("image", event.target.value)}
            required
          />
        </label>
        <p className="text-xs text-muted-foreground">{t("orchestration:maintenanceSandboxHint")}</p>
        <GrantTextField
          label={t("orchestration:maintenancePositive")}
          value={form.positive}
          onChange={form.setPositive}
          testId="maintenance-positive"
        />
        <GrantTextField
          label={t("orchestration:maintenanceNegative")}
          value={form.negative}
          onChange={form.setNegative}
          testId="maintenance-negative"
        />
        <GrantExpiry hours={form.hours} onChange={form.setHours} />
        {invalid && (
          <p role="alert" className="text-sm">
            {t("orchestration:maintenanceInvalidGrant")}
          </p>
        )}
        <Button
          type="submit"
          disabled={Boolean(options.error)}
          className="cursor-pointer max-md:min-h-11"
        >
          {t("orchestration:maintenanceGrant")}
        </Button>
      </fieldset>
    </form>
  );
}

function GrantTextField({
  label,
  value,
  onChange,
  testId,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  testId: string;
}) {
  return (
    <label className="block text-sm space-y-1">
      <span>{label}</span>
      <Textarea
        value={value}
        onChange={(event) => onChange(event.target.value)}
        data-testid={testId}
        required
      />
    </label>
  );
}

function grantScope(
  draft: MaintenanceScope,
  files: string,
  positive: string,
  negative: string,
): MaintenanceScope | null {
  try {
    const paths = files
      .split("\n")
      .map((path) => path.trim())
      .filter(Boolean);
    const positiveCheck: unknown = JSON.parse(positive),
      negativeCheck: unknown = JSON.parse(negative);
    const valid = (value: unknown): value is string[] =>
      Array.isArray(value) &&
      value.length > 0 &&
      value.length <= 32 &&
      value.every((arg) => typeof arg === "string") &&
      Boolean(value[0]);
    if (
      !valid(positiveCheck) ||
      !valid(negativeCheck) ||
      paths.length === 0 ||
      paths.length > 32 ||
      new Set(paths).size !== paths.length ||
      !draft.repository_id ||
      !draft.workflow_id ||
      !draft.workflow_step_id ||
      !draft.profile_id ||
      !draft.image
    )
      return null;
    return {
      ...draft,
      actions: ["read", "patch", "test", "commit"],
      files: paths,
      positive_check: positiveCheck,
      negative_check: negativeCheck,
    };
  } catch {
    return null;
  }
}

function GrantExpiry({ hours, onChange }: { hours: string; onChange: (value: string) => void }) {
  const { t } = useTranslation();
  return (
    <label className="block text-sm space-y-1">
      <span>{t("orchestration:maintenanceExpiry")}</span>
      <Input
        type="number"
        min={1}
        max={168}
        value={hours}
        onChange={(event) => onChange(event.target.value)}
        required
      />
    </label>
  );
}

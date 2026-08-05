import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { SSHIdentitySource } from "@/lib/types/http-ssh";
import type { SSHExecutorConfig } from "./ssh-connection-card";

type FieldOnChange = <K extends keyof SSHExecutorConfig>(
  key: K,
  value: SSHExecutorConfig[K],
) => void;

type SSHConnectionFormProps = {
  form: SSHExecutorConfig;
  baseline: SSHExecutorConfig;
  onChange: FieldOnChange;
};

function fieldIsDirty<K extends keyof SSHExecutorConfig>(
  form: SSHExecutorConfig,
  baseline: SSHExecutorConfig,
  key: K,
  fallback: NonNullable<SSHExecutorConfig[K]>,
): boolean {
  return (form[key] ?? fallback) !== (baseline[key] ?? fallback);
}

export function SSHConnectionForm({ form, baseline, onChange }: SSHConnectionFormProps) {
  const { t } = useTranslation();
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
      <TextField
        id="ssh-name"
        testId="ssh-input-name"
        label={t("executors:name")}
        placeholder={t("executors:sshNamePlaceholder")}
        value={form.name}
        isDirty={fieldIsDirty(form, baseline, "name", "")}
        onChange={(value) => onChange("name", value)}
      />
      <TextField
        id="ssh-host-alias"
        testId="ssh-input-host-alias"
        label={t("executors:sshHostAliasLabel", { path: "~/.ssh/config" })}
        hint={t("executors:sshHostAliasHint")}
        placeholder="prod"
        value={form.host_alias ?? ""}
        isDirty={fieldIsDirty(form, baseline, "host_alias", "")}
        onChange={(value) => onChange("host_alias", value)}
      />
      <TextField
        id="ssh-host"
        testId="ssh-input-host"
        label={t("executors:host")}
        placeholder="dev.example.com"
        value={form.host ?? ""}
        isDirty={fieldIsDirty(form, baseline, "host", "")}
        onChange={(value) => onChange("host", value)}
      />
      <TextField
        id="ssh-port"
        testId="ssh-input-port"
        label={t("executors:port")}
        type="number"
        placeholder="22"
        value={String(form.port ?? 22)}
        isDirty={fieldIsDirty(form, baseline, "port", 22)}
        onChange={(value) => onChange("port", parseInt(value, 10) || 22)}
      />
      <TextField
        id="ssh-user"
        testId="ssh-input-user"
        label={t("executors:user")}
        placeholder="ubuntu"
        value={form.user ?? ""}
        isDirty={fieldIsDirty(form, baseline, "user", "")}
        onChange={(value) => onChange("user", value)}
      />
      <IdentitySourceField
        value={form.identity_source}
        isDirty={fieldIsDirty(form, baseline, "identity_source", "agent")}
        onChange={(value) => onChange("identity_source", value)}
      />
      {form.identity_source === "file" && (
        <TextField
          id="ssh-identity-file"
          testId="ssh-input-identity-file"
          label={t("executors:sshIdentityFilePath")}
          hint={t("executors:sshIdentityFileHint")}
          placeholder="~/.ssh/id_ed25519"
          value={form.identity_file ?? ""}
          isDirty={fieldIsDirty(form, baseline, "identity_file", "")}
          onChange={(value) => onChange("identity_file", value)}
        />
      )}
      <TextField
        id="ssh-proxy-jump"
        testId="ssh-input-proxy-jump"
        label={t("executors:sshProxyJumpLabel")}
        hint={t("executors:sshProxyJumpHint")}
        placeholder="bastion.example.com"
        value={form.proxy_jump ?? ""}
        isDirty={fieldIsDirty(form, baseline, "proxy_jump", "")}
        onChange={(value) => onChange("proxy_jump", value)}
      />
    </div>
  );
}

type TextFieldProps = {
  id: string;
  testId: string;
  label: string;
  hint?: string;
  placeholder?: string;
  type?: string;
  value: string;
  isDirty: boolean;
  onChange: (value: string) => void;
};

function TextField({
  id,
  testId,
  label,
  hint,
  placeholder,
  type,
  value,
  isDirty,
  onChange,
}: TextFieldProps) {
  return (
    <FieldShell id={id} label={label} hint={hint}>
      <Input
        id={id}
        data-testid={testId}
        type={type}
        value={value}
        data-settings-dirty={isDirty}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
      />
    </FieldShell>
  );
}

function IdentitySourceField({
  value,
  isDirty,
  onChange,
}: {
  value: SSHIdentitySource;
  isDirty: boolean;
  onChange: (value: SSHIdentitySource) => void;
}) {
  const { t } = useTranslation();
  return (
    <FieldShell id="ssh-identity-source" label={t("executors:sshIdentitySource")}>
      <Select value={value} onValueChange={(next) => onChange(next as SSHIdentitySource)}>
        <SelectTrigger
          id="ssh-identity-source"
          data-testid="ssh-input-identity-source"
          data-settings-dirty={isDirty}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="agent" data-testid="ssh-input-identity-source-agent">
            {/* The OpenSSH agent program and the environment variable it
                publishes its socket on are identifiers the user finds under
                those names on their own machine, so both travel as interpolated
                values and the pseudo-locale leaves them intact. Only the
                bracketing around them is copy. */}
            {t("executors:sshIdentitySourceAgent", {
              program: "ssh-agent",
              envVar: "SSH_AUTH_SOCK",
            })}
          </SelectItem>
          <SelectItem value="file" data-testid="ssh-input-identity-source-file">
            {t("executors:sshIdentitySourceFile")}
          </SelectItem>
        </SelectContent>
      </Select>
    </FieldShell>
  );
}

function FieldShell({
  id,
  label,
  hint,
  children,
}: {
  id: string;
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}

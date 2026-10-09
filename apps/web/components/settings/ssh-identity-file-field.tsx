"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { SSHFieldShell } from "@/components/settings/ssh-field-shell";
import { listSSHIdentities } from "@/lib/api/domains/ssh-api";
import type { SSHIdentity } from "@/lib/types/http-ssh";

// i18n-exempt: control-flow sentinel for the "type a path instead" row,
// compared with === and never rendered as copy.
const CUSTOM_PATH = "__custom_path__";

const FIELD_ID = "ssh-identity-file";
const FREE_TEXT_TEST_ID = "ssh-input-identity-file";
const SELECT_TEST_ID = "ssh-identity-file-select";

type DiscoveryState =
  | { status: "loading" }
  | { status: "ready"; identities: SSHIdentity[] }
  | { status: "unavailable" };

/**
 * Identity file chooser for the SSH connection form.
 *
 * The value names a file on the machine running the backend, not on the user's
 * own workstation and not on the remote host. That distinction is invisible in
 * a headless install, so the field offers what the backend can actually see and
 * keeps free-text entry for everything it cannot.
 *
 * Discovery is admin-gated and best-effort. Every failure path renders the
 * plain input this field replaces, so losing discovery costs no capability.
 */
export function SSHIdentityFileField({
  value,
  isDirty,
  onChange,
}: {
  value: string;
  isDirty: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const [discovery, setDiscovery] = useState<DiscoveryState>({ status: "loading" });
  const [customMode, setCustomMode] = useState(false);

  useEffect(() => {
    let active = true;
    listSSHIdentities()
      .then((response) => {
        if (!active) return;
        setDiscovery(
          response.identities.length > 0
            ? { status: "ready", identities: response.identities }
            : { status: "unavailable" },
        );
      })
      .catch(() => {
        if (active) setDiscovery({ status: "unavailable" });
      });
    return () => {
      active = false;
    };
  }, []);

  const identities = discovery.status === "ready" ? discovery.identities : [];

  // A saved path discovery did not return is a real configuration, not a
  // mistake: start in custom mode rather than repointing it at a key that
  // happens to be present.
  const valueIsDiscovered = identities.some((identity) => identity.display_path === value);
  useEffect(() => {
    if (discovery.status === "ready" && value !== "" && !valueIsDiscovered) {
      setCustomMode(true);
    }
  }, [discovery.status, value, valueIsDiscovered]);

  const options = useMemo<ComboboxOption[]>(() => {
    const rows: ComboboxOption[] = identities.map((identity) => ({
      value: identity.display_path,
      label: identity.display_path,
      keywords: [identity.display_path, identity.key_type ?? ""],
      description: identity.encrypted
        ? t("executors:sshIdentityEncrypted")
        : (identity.key_type ?? undefined),
      disabled: identity.encrypted,
      disabledReason: identity.encrypted ? t("executors:sshIdentityEncryptedReason") : undefined,
    }));
    rows.push({ value: CUSTOM_PATH, label: t("executors:sshIdentityCustomPath") });
    return rows;
  }, [identities, t]);

  const hint = t("executors:sshIdentityFileBackendHint");

  if (discovery.status !== "ready") {
    return (
      <SSHFieldShell
        id={FIELD_ID}
        label={t("executors:sshIdentityFilePath")}
        hint={discovery.status === "loading" ? t("executors:sshIdentityLoading") : hint}
      >
        <FreeTextPath value={value} isDirty={isDirty} onChange={onChange} />
      </SSHFieldShell>
    );
  }

  return (
    <SSHFieldShell id={FIELD_ID} label={t("executors:sshIdentityFilePath")} hint={hint}>
      <div className="space-y-1.5">
        <Combobox
          options={options}
          value={customMode ? CUSTOM_PATH : value}
          onValueChange={(next) => {
            if (next === CUSTOM_PATH) {
              setCustomMode(true);
              return;
            }
            setCustomMode(false);
            onChange(next);
          }}
          ariaLabel={t("executors:sshIdentityFilePath")}
          placeholder={t("executors:sshIdentitySelectPlaceholder")}
          searchPlaceholder={t("executors:sshIdentitySearchPlaceholder")}
          emptyMessage={t("executors:sshIdentityNoMatch")}
          testId={SELECT_TEST_ID}
        />
        {customMode && <FreeTextPath value={value} isDirty={isDirty} onChange={onChange} />}
      </div>
    </SSHFieldShell>
  );
}

function FreeTextPath({
  value,
  isDirty,
  onChange,
}: {
  value: string;
  isDirty: boolean;
  onChange: (value: string) => void;
}) {
  return (
    <Input
      id={FIELD_ID}
      data-testid={FREE_TEXT_TEST_ID}
      value={value}
      data-settings-dirty={isDirty}
      placeholder="~/.ssh/id_ed25519"
      onChange={(event) => onChange(event.target.value)}
    />
  );
}

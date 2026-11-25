"use client";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { generateUUID } from "@/lib/uuid";
import type { CreateSSHIdentity, SSHIdentity } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { settingsActionClassName, settingsControlClassName } from "../settings-control";
export function IdentityForm({
  manager,
  onCreated,
  onCancel,
}: {
  manager: SSHManager;
  onCreated: (identity: SSHIdentity) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const [kind, setKind] = useState<CreateSSHIdentity["kind"]>("generate");
  const [label, setLabel] = useState("");
  const [privateKey, setPrivateKey] = useState("");
  const [passphrase, setPassphrase] = useState("");
  const [reference, setReference] = useState("");
  const [requestId, setRequestId] = useState(generateUUID);
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  const changed = () => setRequestId(generateUUID());
  const submit = async () => {
    try {
      const identity = await manager.createIdentity({
        request_id: requestId,
        label,
        kind,
        private_key: privateKey,
        passphrase,
        reference,
      });
      if (active.current) onCreated(identity);
    } catch {
      /* The manager renders a sanitized error. */
    } finally {
      setPrivateKey("");
      setPassphrase("");
    }
  };
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        void submit();
      }}
      className="space-y-4"
    >
      <fieldset disabled={manager.busy} className="space-y-4">
        <legend className="text-sm font-medium">{t("settings:sshManager.newIdentity")}</legend>
        <IdentityLabel
          label={label}
          onChange={(value) => {
            setLabel(value);
            changed();
          }}
        />
        <IdentityKind
          value={kind}
          onChange={(value) => {
            setKind(value);
            setPrivateKey("");
            setPassphrase("");
            setReference("");
            changed();
          }}
        />
        <IdentityMaterialFields
          kind={kind}
          privateKey={privateKey}
          passphrase={passphrase}
          reference={reference}
          setPrivateKey={setPrivateKey}
          setPassphrase={setPassphrase}
          setReference={setReference}
          changed={changed}
        />
        <p className="text-sm text-muted-foreground">{t("settings:sshManager.custody")}</p>
        <div className="flex flex-wrap gap-2">
          <Button type="submit" className={settingsActionClassName()}>
            {t("settings:sshManager.createIdentity")}
          </Button>
          <Button
            type="button"
            variant="outline"
            className={settingsActionClassName()}
            onClick={onCancel}
          >
            {t("common:cancel")}
          </Button>
        </div>
      </fieldset>
    </form>
  );
}

function IdentityMaterialFields({
  kind,
  privateKey,
  passphrase,
  reference,
  setPrivateKey,
  setPassphrase,
  setReference,
  changed,
}: {
  kind: CreateSSHIdentity["kind"];
  privateKey: string;
  passphrase: string;
  reference: string;
  setPrivateKey: (value: string) => void;
  setPassphrase: (value: string) => void;
  setReference: (value: string) => void;
  changed: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {" "}
      {kind === "import" && (
        <>
          <div className="space-y-1">
            <Label htmlFor="ssh-private-key">{t("settings:sshManager.privateKey")}</Label>
            <Textarea
              id="ssh-private-key"
              autoComplete="off"
              spellCheck={false}
              value={privateKey}
              onChange={(e) => {
                setPrivateKey(e.target.value);
                changed();
              }}
              required
              maxLength={65536}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="ssh-key-passphrase">{t("settings:sshManager.passphrase")}</Label>
            <Input
              id="ssh-key-passphrase"
              className={settingsControlClassName()}
              type="password"
              autoComplete="new-password"
              value={passphrase}
              onChange={(e) => setPassphrase(e.target.value)}
            />
          </div>
        </>
      )}
      {(kind === "file" || kind === "agent") && (
        <div className="space-y-1">
          <Label htmlFor="ssh-reference">{t("settings:sshManager.reference")}</Label>
          <Input
            id="ssh-reference"
            className={settingsControlClassName()}
            value={reference}
            onChange={(e) => {
              setReference(e.target.value);
              changed();
            }}
            required
          />
        </div>
      )}
    </>
  );
}

function IdentityLabel({ label, onChange }: { label: string; onChange: (value: string) => void }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1">
      <Label htmlFor="ssh-identity-label">{t("settings:sshManager.label")}</Label>
      <Input
        id="ssh-identity-label"
        className={settingsControlClassName()}
        value={label}
        onChange={(event) => onChange(event.target.value)}
        required
        maxLength={200}
      />
    </div>
  );
}

function IdentityKind({
  value,
  onChange,
}: {
  value: CreateSSHIdentity["kind"];
  onChange: (value: CreateSSHIdentity["kind"]) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1">
      <Label htmlFor="ssh-identity-kind">{t("settings:sshManager.method")}</Label>
      <select
        id="ssh-identity-kind"
        className={settingsControlClassName("w-full rounded-md border bg-background px-2")}
        value={value}
        onChange={(event) => onChange(event.target.value as CreateSSHIdentity["kind"])}
      >
        <option value="generate">{t("settings:sshManager.generate")}</option>
        <option value="import">{t("settings:sshManager.import")}</option>
        <option value="file">{t("settings:sshManager.file")}</option>
        <option value="agent">{t("settings:sshManager.agent")}</option>
      </select>
    </div>
  );
}

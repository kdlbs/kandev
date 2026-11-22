"use client";
import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { IconKey, IconLock, IconTerminal2 } from "@tabler/icons-react";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import { cn } from "@/lib/utils";
import { settingsControlClassName } from "../settings-control";
import { ConnectionPicker } from "./picker";
import { PublicKeyDetails } from "./public-key-details";
export type EnrollmentMethod = "" | "password" | "identity" | "manual";
export function EnrollmentMethods({
  method,
  onMethod,
  password,
  onPassword,
  bootstrap,
  onBootstrap,
  manager,
  publicKey,
}: {
  method: EnrollmentMethod;
  onMethod: (value: EnrollmentMethod) => void;
  password: string;
  onPassword: (value: string) => void;
  bootstrap: string;
  onBootstrap: (value: string) => void;
  manager: SSHManager;
  publicKey: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-5">
      <div role="radiogroup" aria-label={t("settings:sshManager.method")} className="space-y-2">
        {(
          [
            { value: "password", key: "methodPassword", help: "passwordHelp", Icon: IconLock },
            { value: "identity", key: "methodIdentity", help: "identityHelp", Icon: IconKey },
            { value: "manual", key: "methodManual", help: "manualHelp", Icon: IconTerminal2 },
          ] as const
        ).map(({ value, key, help, Icon }) => (
          <label
            key={value}
            className={cn(
              "flex cursor-pointer items-start gap-3 rounded-lg border p-3 transition-colors hover:bg-muted/50 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring motion-reduce:transition-none",
              method === value && "border-primary/50 bg-primary/5",
            )}
          >
            <input
              type="radio"
              name="ssh-install-method"
              value={value}
              checked={method === value}
              onChange={() => onMethod(value)}
              aria-label={t(`settings:sshManager.experience.${key}`)}
              className="mt-1 size-4 accent-primary"
            />
            <Icon aria-hidden className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
            <span className="space-y-1">
              <span className="block text-sm font-medium">
                {t(`settings:sshManager.experience.${key}`)}
              </span>
              <span className="block text-xs leading-relaxed text-muted-foreground">
                {t(`settings:sshManager.experience.${help}`)}
              </span>
            </span>
          </label>
        ))}
      </div>
      {method === "password" && (
        <div className="space-y-2">
          <Label htmlFor="ssh-enrollment-password">
            {t("settings:sshManager.enrollment.password")}
          </Label>
          <Input
            id="ssh-enrollment-password"
            className={settingsControlClassName()}
            type="password"
            autoComplete="off"
            value={password}
            onChange={(event) => onPassword(event.target.value)}
          />
          <p className="text-xs leading-relaxed text-muted-foreground">
            {t("settings:sshManager.enrollment.passwordHelp")}
          </p>
        </div>
      )}
      {method === "identity" && (
        <ConnectionPicker
          id="ssh-enrollment-identity"
          label={t("settings:sshManager.enrollment.existingIdentity")}
          value={bootstrap}
          onChange={onBootstrap}
          disabled={manager.busy}
          options={[
            { value: "", label: t("settings:sshManager.selectIdentity") },
            ...manager.identities.map((item) => ({ value: item.id, label: item.label })),
          ]}
        />
      )}
      {method === "manual" && (
        <div className="space-y-3">
          <p className="text-sm leading-relaxed text-muted-foreground">
            {t("settings:sshManager.enrollment.manual")}
          </p>
          <PublicKeyDetails publicKey={publicKey} />
        </div>
      )}
    </div>
  );
}

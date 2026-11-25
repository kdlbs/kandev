"use client";
import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import {
  IconAlertTriangle,
  IconCheck,
  IconCopy,
  IconLoader2,
  IconInfoCircle,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
import { settingsActionClassName } from "../settings-control";

export function AccessHeading({ title, description }: { title: string; description?: ReactNode }) {
  return (
    <div className="space-y-2">
      <h3
        tabIndex={-1}
        data-ssh-step-heading
        className="text-base font-semibold tracking-tight outline-none"
      >
        {title}
      </h3>
      {description && (
        <p className="max-w-prose text-sm leading-relaxed text-muted-foreground">{description}</p>
      )}
    </div>
  );
}
export function AccessStatus({
  tone = "neutral",
  title,
  children,
  pending = false,
  staticMessage = false,
}: {
  tone?: "neutral" | "success" | "warning" | "error";
  title: string;
  children?: ReactNode;
  pending?: boolean;
  staticMessage?: boolean;
}) {
  let Icon = tone === "neutral" ? IconInfoCircle : IconAlertTriangle;
  if (pending) Icon = IconLoader2;
  else if (tone === "success") Icon = IconCheck;
  const role = tone === "error" ? "alert" : "status";
  return (
    <div role={staticMessage ? undefined : role} className="flex min-w-0 items-start gap-2">
      <span
        className={cn(
          "flex size-5 shrink-0 items-center justify-center text-muted-foreground",
          tone === "success" && "text-green-800 dark:text-green-300",
          tone === "warning" && "text-amber-700 dark:text-amber-300",
          tone === "error" && "text-destructive",
        )}
      >
        <Icon aria-hidden className={cn("size-4", pending && "motion-safe:animate-spin")} />
      </span>
      <div className="min-w-0 space-y-1">
        <p className="text-sm font-medium leading-5">{title}</p>
        {children && (
          <div className="text-sm leading-relaxed text-muted-foreground">{children}</div>
        )}
      </div>
    </div>
  );
}
export function EvidenceRow({
  label,
  value,
  copy = false,
}: {
  label: string;
  value: string;
  copy?: boolean;
}) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState<boolean | null>(null);
  return (
    <div className="grid min-w-0 gap-1.5 py-3 md:grid-cols-[9rem_minmax(0,1fr)] md:gap-4">
      <dt className="text-xs font-medium text-muted-foreground md:pt-1">{label}</dt>
      <dd className="min-w-0 space-y-1">
        <div className="flex min-w-0 items-start gap-2">
          <span className="min-w-0 flex-1 break-all font-mono text-xs leading-6">{value}</span>
          {copy && (
            <Button
              variant="ghost"
              size="icon"
              aria-label={t("settings:sshManager.experience.copyValue", { label })}
              className={settingsActionClassName(
                "shrink-0 size-7 max-md:size-11 [@media(pointer:coarse)]:size-11",
              )}
              onClick={() => {
                void copyToClipboard(value).then(setCopied);
              }}
            >
              <IconCopy aria-hidden className="size-3.5" />
            </Button>
          )}
        </div>
        {copied !== null && (
          <p role="status" className="text-xs text-muted-foreground">
            {t(copied ? "settings:sshManager.copied" : "settings:sshManager.copyFailed")}
          </p>
        )}
      </dd>
    </div>
  );
}

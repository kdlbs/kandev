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

export function AccessSurface({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("min-w-0 rounded-xl border bg-card p-4 md:p-5", className)}>{children}</div>
  );
}
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
        <p className="text-sm leading-relaxed text-muted-foreground">{description}</p>
      )}
    </div>
  );
}
export function AccessStatus({
  tone = "neutral",
  title,
  children,
  pending = false,
}: {
  tone?: "neutral" | "success" | "warning" | "error";
  title: string;
  children?: ReactNode;
  pending?: boolean;
}) {
  let Icon = tone === "neutral" ? IconInfoCircle : IconAlertTriangle;
  if (pending) Icon = IconLoader2;
  else if (tone === "success") Icon = IconCheck;
  return (
    <div role={tone === "error" ? "alert" : "status"} className="flex min-w-0 items-start gap-3">
      <span
        className={cn(
          "flex size-8 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground",
          tone === "success" && "bg-success/10 text-green-800 dark:text-green-300",
          tone === "warning" && "bg-amber-500/10 text-amber-700 dark:text-amber-300",
          tone === "error" && "bg-destructive/10 text-destructive",
        )}
      >
        <Icon aria-hidden className={cn("size-4", pending && "motion-safe:animate-spin")} />
      </span>
      <div className="min-w-0 space-y-1 pt-1">
        <p className="text-sm font-medium">{title}</p>
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

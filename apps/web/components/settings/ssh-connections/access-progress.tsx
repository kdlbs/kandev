"use client";
import { useTranslation } from "react-i18next";
import { IconCheck } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { ConnectionState } from "./connection-presentation";

export function AccessProgress({ state }: { state: ConnectionState }) {
  const { t } = useTranslation();
  let current = 2;
  if (state === "incomplete" || state === "unavailable") current = 0;
  else if (state === "untrusted") current = 1;
  const steps = ["details", "serverTrust", "sshAccess"];
  return (
    <div>
      <ol
        className="hidden items-center gap-4 md:flex"
        aria-label={t("settings:sshManager.ui.setupAccess")}
      >
        {steps.map((step, index) => (
          <li
            key={step}
            aria-current={index === current ? "step" : undefined}
            className={cn(
              "flex min-w-0 flex-1 items-center gap-2.5 text-xs",
              index <= current ? "text-foreground" : "text-muted-foreground",
            )}
          >
            <span
              className={cn(
                "flex size-6 shrink-0 items-center justify-center rounded-full border text-[11px] tabular-nums",
                index === current && "border-primary/40 bg-primary/10 text-primary",
              )}
            >
              {index < current || state === "ready" ? (
                <IconCheck aria-hidden className="size-3.5" />
              ) : (
                index + 1
              )}
            </span>
            <span>{t(`settings:sshManager.experience.${step}`)}</span>
          </li>
        ))}
      </ol>
      <p className="text-xs font-medium text-muted-foreground md:hidden">
        {t("settings:sshManager.experience.step", { current: current + 1, total: 3 })}
        <span className="mx-2" aria-hidden>
          ·
        </span>
        {t(`settings:sshManager.experience.${steps[current]}`)}
      </p>
    </div>
  );
}

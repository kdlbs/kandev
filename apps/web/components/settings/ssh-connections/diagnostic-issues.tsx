"use client";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { IconArrowRight } from "@tabler/icons-react";
import type { SSHPreflight } from "@/lib/types/http-ssh-connections";
import { settingsActionClassName } from "../settings-control";
import { diagnosticIssue, type SSHHop, type SSHRemedy } from "./diagnostic-presentation";
export type RemedyAction = (remedy: SSHRemedy, hop?: SSHHop) => void;
export function DiagnosticIssues({
  checks,
  onRemedy,
}: {
  checks: SSHPreflight["checks"];
  onRemedy?: RemedyAction;
}) {
  const { t } = useTranslation();
  const issues = checks.map((check) => diagnosticIssue(check.code, check.hop));
  return (
    <div className="space-y-6">
      {(["bastion", "destination", undefined] as const).map((hop) => {
        const group = issues.filter((issue) => issue.hop === hop);
        if (!group.length) return null;
        return (
          <section key={hop ?? "unknown"} className="space-y-2">
            <h4 className="text-xs font-semibold text-muted-foreground">
              {t(hop ? `settings:sshManager.${hop}` : "settings:sshManager.experience.setup")}
            </h4>
            <ul className="divide-y divide-border/60">
              {group.map((issue, index) => (
                <li key={`${issue.code}:${index}`} className="space-y-3 py-4 first:pt-1">
                  <p className="text-sm leading-relaxed">
                    {t(`settings:sshManager.errors.${issue.code}`)}
                  </p>
                  {issue.remedy && onRemedy && (
                    <Button
                      variant="outline"
                      className={settingsActionClassName("max-md:w-full")}
                      onClick={() => onRemedy(issue.remedy!, issue.hop)}
                    >
                      {t(
                        `settings:sshManager.experience.${{ edit: "remedyEdit", access: "remedyAccess", identity: "remedyIdentity" }[issue.remedy]}`,
                      )}
                      <IconArrowRight aria-hidden className="ml-1.5 size-3.5" />
                    </Button>
                  )}
                  {issue.technicalCode && (
                    <details>
                      <summary className="inline-flex min-h-7 cursor-pointer items-center text-xs text-muted-foreground max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
                        {t("settings:sshManager.experience.technicalDetails")}
                      </summary>
                      <code className="block break-all text-xs text-muted-foreground">
                        {issue.technicalCode}
                      </code>
                    </details>
                  )}
                </li>
              ))}
            </ul>
          </section>
        );
      })}
    </div>
  );
}

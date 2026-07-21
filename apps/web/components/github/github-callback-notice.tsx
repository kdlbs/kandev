"use client";

import { IconAlertTriangle, IconCheck, IconInfoCircle } from "@tabler/icons-react";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { useSearchParams } from "@/lib/routing/client-router";
import { parseGitHubCallbackResult } from "@/hooks/domains/github/use-github-app-registrations";
import { callbackNotice } from "./github-app-onboarding-model";
import { cn } from "@/lib/utils";

export function GitHubCallbackNotice({ workspaceId }: { workspaceId: string }) {
  const search = useSearchParams();
  const result = parseGitHubCallbackResult(search, workspaceId);
  if (!result) return null;
  const notice = callbackNotice(result.code);
  const Icon = noticeIcon(notice.tone);
  return (
    <Alert
      className={cn(
        notice.tone === "success" && "border-green-500/40",
        notice.tone === "error" && "border-destructive/50",
      )}
      data-testid="github-callback-notice"
    >
      <Icon className="h-4 w-4" />
      <AlertTitle>{notice.title}</AlertTitle>
      <AlertDescription>{notice.description}</AlertDescription>
    </Alert>
  );
}

function noticeIcon(tone: "success" | "info" | "error") {
  if (tone === "success") return IconCheck;
  if (tone === "info") return IconInfoCircle;
  return IconAlertTriangle;
}

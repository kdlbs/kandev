"use client";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconCopy } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
import { settingsActionClassName } from "../settings-control";
export function PublicKeyDetails({ publicKey }: { publicKey: string }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState<boolean | null>(null);
  if (!publicKey) return null;
  return (
    <div className="space-y-3 min-w-0">
      <Button
        variant="outline"
        className={settingsActionClassName()}
        onClick={() => {
          void copyToClipboard(publicKey).then(setCopied);
        }}
      >
        <IconCopy className="size-3.5 mr-1.5" aria-hidden />
        {t("settings:sshManager.copyPublicKey")}
      </Button>
      {copied !== null && (
        <p role="status" className="text-sm text-muted-foreground">
          {t(copied ? "settings:sshManager.copied" : "settings:sshManager.copyFailed")}
        </p>
      )}
      <details className="min-w-0">
        <summary className="w-fit text-xs text-muted-foreground cursor-pointer inline-flex items-center min-h-7 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
          {t("settings:sshManager.ui.showPublicKey")}
        </summary>
        <pre className="mt-2 whitespace-pre-wrap break-all rounded-md bg-muted p-3 text-xs">
          {publicKey}
        </pre>
      </details>
    </div>
  );
}

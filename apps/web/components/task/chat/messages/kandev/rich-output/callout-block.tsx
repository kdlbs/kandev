import { useTranslation } from "react-i18next";
import {
  IconInfoCircle,
  IconCircleCheck,
  IconAlertTriangle,
  IconCircleX,
} from "@tabler/icons-react";
import { Alert, AlertTitle, AlertDescription } from "@kandev/ui/alert";
import type { CalloutProps } from "@/lib/rich-output/catalog";

const kinds = {
  info: { Icon: IconInfoCircle, key: "task:richOutputCalloutInfo" },
  success: { Icon: IconCircleCheck, key: "task:richOutputCalloutSuccess" },
  warning: { Icon: IconAlertTriangle, key: "task:richOutputCalloutWarning" },
  error: { Icon: IconCircleX, key: "task:richOutputCalloutError" },
} as const;

export function Callout({ kind, title, text }: CalloutProps) {
  const { t } = useTranslation();
  const { Icon, key } = kinds[kind];
  return (
    <Alert
      role="note"
      className="min-w-0"
      variant={kind === "error" ? "destructive" : "default"}
      data-testid="rich-output-callout"
    >
      <Icon aria-hidden />
      <AlertTitle className="whitespace-pre-wrap">
        <span data-callout-label>{t(key)}</span>
        {title && <p className="mt-1">{title}</p>}
      </AlertTitle>
      <AlertDescription className="whitespace-pre-wrap">{text}</AlertDescription>
    </Alert>
  );
}

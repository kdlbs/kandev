import { IconCheck, IconCopy } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";

export function CommitCopyIdentifier({ sha }: { sha: string }) {
  const { t } = useTranslation();
  const { copied, copy } = useCopyToClipboard();

  return (
    <span className="inline-flex max-w-full items-center align-middle">
      <code className="font-mono text-[11px]">{sha.slice(0, 7)}</code>
      <button
        type="button"
        aria-label={t("task:copyIdentifier")}
        title={t("task:copyIdentifier")}
        onClick={() => void copy(sha)}
        className="inline-flex size-7 shrink-0 cursor-pointer items-center justify-center rounded hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring max-md:size-11 [@media(pointer:coarse)]:size-11"
      >
        {copied ? (
          <IconCheck size={14} aria-hidden="true" />
        ) : (
          <IconCopy size={14} aria-hidden="true" />
        )}
      </button>
      <span role="status" className="sr-only">
        {copied ? t("task:copied") : ""}
      </span>
    </span>
  );
}

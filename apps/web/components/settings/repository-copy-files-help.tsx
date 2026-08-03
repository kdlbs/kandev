"use client";

import { Trans, useTranslation } from "react-i18next";
import { IconChevronDown, IconInfoCircle } from "@tabler/icons-react";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import type { Repository } from "@/lib/types/http";

/**
 * Glob/path syntax the backend parses and the user types verbatim. These are
 * VALUES, not copy, so each is interpolated into its message rather than left in
 * the tag body — the pseudo-locale would otherwise accent `**` or `:symlink`
 * into a pattern that no longer matches anything.
 */
const SYMLINK_SUFFIX = ":symlink";
const ESCAPED_SYMLINK_SUFFIX = "::symlink";
const LITERAL_PATTERN = ".env";
const STAR_PATTERN = "*";
const QUESTION_PATTERN = "?";
const CHAR_CLASS_PATTERN = "[abc]";
const GLOBSTAR_PATTERN = "**";
const GLOBSTAR_EXAMPLE = "**/.env";
const BRACES_PATTERN = "{a,b}";
const BRACES_EXAMPLE = ".env{,.local}";
const REMOTE_COPY_SIZE_LIMIT = "5 MiB";
const COPY_FILES_PLACEHOLDER = ".env, .env.*, apps/**/.env, .env.local:symlink";

const codeClass = "px-1 py-0.5 bg-muted rounded";

type CopyFilesFieldProps = {
  repositoryId: string;
  copyFiles: string;
  isDirty?: boolean;
  onUpdate: (repoId: string, updates: Partial<Repository>) => void;
};

export function CopyFilesField({
  repositoryId,
  copyFiles,
  isDirty = false,
  onUpdate,
}: CopyFilesFieldProps) {
  const { t } = useTranslation();
  const inputId = `copy-files-${repositoryId}`;
  const helpId = `copy-files-help-${repositoryId}`;
  return (
    <div className="space-y-2">
      <Label htmlFor={inputId}>{t("workspaces:copyFiles")}</Label>
      {/* The placeholder is a list of glob patterns — a value, not prose. */}
      <Textarea
        id={inputId}
        data-testid={`copy-files-input-${repositoryId}`}
        aria-describedby={helpId}
        value={copyFiles}
        onChange={(e) => onUpdate(repositoryId, { copy_files: e.target.value })}
        placeholder={COPY_FILES_PLACEHOLDER}
        rows={2}
        className="font-mono text-sm"
        data-settings-dirty={isDirty}
      />
      <p id={helpId} className="text-xs text-muted-foreground">
        <Trans
          i18nKey="workspaces:copyFilesHelp"
          values={{ symlink: SYMLINK_SUFFIX, escapedSymlink: ESCAPED_SYMLINK_SUFFIX }}
        >
          <code className={codeClass} />
          <code className={codeClass} />
          <code className={codeClass} />
        </Trans>
      </p>
      <p data-testid="copy-files-remote-fallback" className="text-xs text-muted-foreground">
        {t("workspaces:copyFilesRemoteFallback")}
      </p>
      <CopyFilesDetails />
    </div>
  );
}

function CopyFilesDetails() {
  const { t } = useTranslation();
  return (
    <details className="group text-xs text-muted-foreground">
      <summary className="flex min-h-11 w-fit cursor-pointer list-none items-center gap-1.5 py-2 font-medium text-foreground">
        <IconInfoCircle className="h-4 w-4 shrink-0" aria-hidden="true" />
        {t("workspaces:patternSyntax")}
        <IconChevronDown
          className="h-4 w-4 shrink-0 transition-transform group-open:rotate-180"
          aria-hidden="true"
        />
      </summary>
      <div className="max-w-sm space-y-2 pb-1">
        <p>{t("workspaces:copyFilesPathsResolved")}</p>
        <p className="font-medium">{t("workspaces:supportedPatterns")}</p>
        <ul className="space-y-1 pl-3 list-disc">
          <li>
            <Trans
              i18nKey="workspaces:copyFilesPatternLiteral"
              values={{ pattern: LITERAL_PATTERN }}
            >
              <code className={codeClass} />
            </Trans>
          </li>
          <li>
            <Trans
              i18nKey="workspaces:copyFilesPatternWildcards"
              values={{
                star: STAR_PATTERN,
                question: QUESTION_PATTERN,
                charClass: CHAR_CLASS_PATTERN,
              }}
            >
              <code className={codeClass} />
              <code className={codeClass} />
              <code className={codeClass} />
            </Trans>
          </li>
          <li>
            <Trans
              i18nKey="workspaces:copyFilesPatternGlobstar"
              values={{ globstar: GLOBSTAR_PATTERN, example: GLOBSTAR_EXAMPLE }}
            >
              <code className={codeClass} />
              <code className={codeClass} />
            </Trans>
          </li>
          <li>
            <Trans
              i18nKey="workspaces:copyFilesPatternBraces"
              values={{ braces: BRACES_PATTERN, example: BRACES_EXAMPLE }}
            >
              <code className={codeClass} />
              <code className={codeClass} />
            </Trans>
          </li>
        </ul>
        <p className="text-muted-foreground">
          {t("workspaces:copyFilesSizeLimit", { limit: REMOTE_COPY_SIZE_LIMIT })}
        </p>
      </div>
    </details>
  );
}

import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { useTranslation } from "react-i18next";
import type { CustomPrompt } from "@/lib/types/http";
import { settingsActionClassName, settingsControlClassName } from "./settings-control";

export function filterSavedPrompts(
  prompts: CustomPrompt[],
  query: string,
  editingId: string | null,
  deleteTargetId: string | null,
): CustomPrompt[] {
  const normalized = query.trim().toLowerCase();
  return normalized
    ? prompts.filter(
        (prompt) =>
          prompt.id === editingId ||
          prompt.id === deleteTargetId ||
          [prompt.name, prompt.content].some((value) => value.toLowerCase().includes(normalized)),
      )
    : prompts;
}

export function PromptListSearch({
  query,
  onQueryChange,
  disabled,
}: {
  query: string;
  onQueryChange: (value: string) => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  return (
    <Input
      type="search"
      value={query}
      onChange={(event) => onQueryChange(event.target.value)}
      placeholder={t("common:searchPlaceholder")}
      aria-label={t("common:searchPlaceholder")}
      disabled={disabled}
      className={settingsControlClassName("min-w-0 w-full")}
    />
  );
}

export function PromptCreateButton({
  onClick,
  disabled,
}: {
  onClick: () => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  return (
    <Button
      onClick={onClick}
      disabled={disabled}
      className={settingsActionClassName()}
      data-testid="prompt-create-button"
    >
      {t("settings:promptAdd")}
    </Button>
  );
}

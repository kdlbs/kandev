import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { DropdownMenuItem } from "@kandev/ui/dropdown-menu";

export type SavedQueryLoadState = {
  loading: boolean;
  error: boolean;
  retry: () => void;
};

export function SavedQueryLoadStatus({
  loading,
  error,
  retry,
  presentation = "inline",
}: SavedQueryLoadState & { presentation?: "inline" | "menu" }) {
  const { t } = useTranslation();
  if (loading) {
    return (
      <p role="status" className="px-3 py-2 text-xs text-muted-foreground">
        {t("github:loadingSavedQueries")}
      </p>
    );
  }
  if (!error) return null;
  return (
    <div className="flex flex-wrap items-center gap-2 px-3 py-2">
      <p role="alert" className="text-xs text-destructive">
        {t("github:savedQueriesLoadFailed")}
      </p>
      {presentation === "menu" ? (
        <DropdownMenuItem
          onSelect={(event) => {
            event.preventDefault();
            retry();
          }}
        >
          {t("github:retry")}
        </DropdownMenuItem>
      ) : (
        <Button variant="outline" onClick={retry}>
          {t("github:retry")}
        </Button>
      )}
    </div>
  );
}

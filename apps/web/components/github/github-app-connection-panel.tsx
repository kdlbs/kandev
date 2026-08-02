"use client";

import { useEffect, useState } from "react";
import { IconExternalLink, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { useToast } from "@/components/toast-provider";
import { useGitHubAppRegistrations } from "@/hooks/domains/github/use-github-app-registrations";
import { GitHubAppCreateForm } from "./github-app-create-form";
import { GitHubAppImportForm } from "./github-app-import-form";
import { GitHubAppRegistrationList } from "./github-app-registration-list";
import { useTranslation } from "react-i18next";

type AppView = "choose" | "import" | "create";
type RegistrationHook = ReturnType<typeof useGitHubAppRegistrations>;

export function GitHubAppConnectionPanel({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const registrations = useGitHubAppRegistrations(workspaceId);
  const [view, setView] = useState<AppView>("choose");
  const { selectedId, setSelectedId, selectedRegistration } = useAppRegistrationSelection(
    workspaceId,
    registrations,
  );
  const { toast } = useToast();

  useEffect(() => {
    setView("choose");
  }, [workspaceId]);

  async function install() {
    if (!selectedRegistration || selectedRegistration.status !== "active") return;
    try {
      const response = await registrations.startInstall(selectedRegistration.id);
      const url = response.url ?? response.URL;
      // Surfaced to the user by the catch below, so it is copy.
      if (!url) throw new Error(t("github:githubDidNotReturnAnInstallationUrl"));
      window.location.assign(url);
    } catch (error) {
      toast({
        description: error instanceof Error ? error.message : t("github:appInstallationFailed"),
        variant: "error",
      });
    }
  }

  if (view === "import") {
    return (
      <div className="space-y-4">
        <BackButton onClick={() => setView("choose")} />
        <GitHubAppImportForm
          workspaceId={workspaceId}
          registrations={registrations}
          onImported={(registrationId) => {
            setSelectedId(registrationId);
            setView("choose");
          }}
        />
      </div>
    );
  }
  if (view === "create") {
    return (
      <div className="space-y-4">
        <BackButton onClick={() => setView("choose")} />
        <GitHubAppCreateForm workspaceId={workspaceId} registrations={registrations} />
      </div>
    );
  }
  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-sm font-medium">{t("github:chooseAGithubApp")}</h3>
        <p className="text-xs leading-5 text-muted-foreground">
          {t("github:useAnAppWhenAutomationNeeds")}
        </p>
      </div>
      {registrations.loading ? (
        <div className="flex min-h-11 items-center gap-2 text-sm text-muted-foreground">
          <Spinner className="h-4 w-4" /> {t("github:loadingRegisteredApps")}
        </div>
      ) : (
        <GitHubAppRegistrationList
          registrations={registrations.registrations}
          value={selectedId}
          onChange={setSelectedId}
        />
      )}
      {registrations.error && <p className="text-xs text-destructive">{registrations.error}</p>}
      <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap">
        <Button
          disabled={selectedRegistration?.status !== "active" || registrations.mutating}
          onClick={() => void install()}
          className="h-11 cursor-pointer"
          data-testid="github-app-install-button"
        >
          {registrations.mutating && <Spinner className="mr-2 h-4 w-4" />}
          {t("github:installForThisWorkspace")}
          <IconExternalLink className="ml-2 h-4 w-4" />
        </Button>
        <Button variant="outline" className="h-11 cursor-pointer" onClick={() => setView("import")}>
          <IconPlus className="mr-2 h-4 w-4" /> {t("github:addExistingApp")}
        </Button>
        <Button variant="outline" className="h-11 cursor-pointer" onClick={() => setView("create")}>
          <IconPlus className="mr-2 h-4 w-4" /> {t("github:createNewApp")}
        </Button>
      </div>
      <p className="text-xs leading-5 text-muted-foreground">
        {t("github:aRegistrationCanBeReusedAcross")}
      </p>
    </div>
  );
}

function useAppRegistrationSelection(workspaceId: string, registrations: RegistrationHook) {
  const [selectedId, setSelectedId] = useState("");
  useEffect(() => setSelectedId(""), [workspaceId]);
  useEffect(() => {
    if (!registrations.loaded) return;
    setSelectedId((current) => {
      const currentRegistration = registrations.registrations.find(({ id }) => id === current);
      if (currentRegistration?.status === "active") return current;
      if (registrations.selected?.status === "active") return registrations.selected.id;
      return registrations.registrations.find(({ status }) => status === "active")?.id ?? "";
    });
  }, [registrations.loaded, registrations.registrations, registrations.selected]);
  const selectedRegistration = registrations.registrations.find(({ id }) => id === selectedId);
  return { selectedId, setSelectedId, selectedRegistration };
}

function BackButton({ onClick }: { onClick: () => void }) {
  const { t } = useTranslation();
  return (
    <Button variant="ghost" className="h-11 cursor-pointer px-2" onClick={onClick}>
      {t("github:backToRegisteredApps")}
    </Button>
  );
}

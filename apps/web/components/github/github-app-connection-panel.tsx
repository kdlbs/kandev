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

type AppView = "choose" | "import" | "create";
type RegistrationHook = ReturnType<typeof useGitHubAppRegistrations>;

export function GitHubAppConnectionPanel({ workspaceId }: { workspaceId: string }) {
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
      if (!url) throw new Error("GitHub did not return an installation URL");
      window.location.assign(url);
    } catch (error) {
      toast({
        description: error instanceof Error ? error.message : "App installation failed",
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
        <h3 className="text-sm font-medium">Choose a GitHub App</h3>
        <p className="text-xs leading-5 text-muted-foreground">
          Use an App when automation needs its own identity, short-lived tokens, and
          repository-level installation control. Setup is more involved and requires a publicly
          reachable HTTPS URL.
        </p>
      </div>
      {registrations.loading ? (
        <div className="flex min-h-11 items-center gap-2 text-sm text-muted-foreground">
          <Spinner className="h-4 w-4" /> Loading registered Apps...
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
          Install for this workspace
          <IconExternalLink className="ml-2 h-4 w-4" />
        </Button>
        <Button variant="outline" className="h-11 cursor-pointer" onClick={() => setView("import")}>
          <IconPlus className="mr-2 h-4 w-4" /> Add existing App
        </Button>
        <Button variant="outline" className="h-11 cursor-pointer" onClick={() => setView("create")}>
          <IconPlus className="mr-2 h-4 w-4" /> Create new App
        </Button>
      </div>
      <p className="text-xs leading-5 text-muted-foreground">
        A registration can be reused across workspaces, or you can keep work and personal Apps
        separate. Each workspace selects and installs its own credential.
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
  return (
    <Button variant="ghost" className="h-11 cursor-pointer px-2" onClick={onClick}>
      Back to registered Apps
    </Button>
  );
}

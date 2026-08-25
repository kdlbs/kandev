"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Checkbox } from "@kandev/ui/checkbox";
import { toast } from "@/lib/toast/sonner";
import { createWorkspaceCoordinatorGrant } from "@/lib/api/domains/coordinator-api";
import type { CreateGrantRequest } from "@/lib/api/domains/coordinator-api";

type Props = {
  workspaceId: string;
  onCreated: () => void;
};

type GrantFormFieldsProps = {
  coordinatorTaskId: string;
  setCoordinatorTaskId: (v: string) => void;
  scopeKind: "workspace" | "workflow";
  setScopeKind: (v: "workspace" | "workflow") => void;
  capInsp: boolean;
  setCapInsp: (v: boolean) => void;
  capOrch: boolean;
  setCapOrch: (v: boolean) => void;
  note: string;
  setNote: (v: string) => void;
};

function GrantFormFields(props: GrantFormFieldsProps) {
  const { t } = useTranslation();
  return (
    <>
      <div className="space-y-2">
        <Label htmlFor="coordinator-task-id">{t("workspaces:coordinatorTaskId")}</Label>
        <Input
          id="coordinator-task-id"
          placeholder="task-uuid"
          value={props.coordinatorTaskId}
          onChange={(e) => props.setCoordinatorTaskId(e.target.value)}
          data-testid="grant-task-id-input"
        />
        <p className="text-xs text-muted-foreground">
          {t("workspaces:coordinatorTaskIdDescription")}
        </p>
      </div>
      <div className="space-y-2">
        <Label>{t("workspaces:scopeKind")}</Label>
        <Select
          value={props.scopeKind}
          onValueChange={(v: "workspace" | "workflow") => props.setScopeKind(v)}
        >
          <SelectTrigger data-testid="grant-scope-select">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="workspace">{t("workspaces:scopeWorkspace")}</SelectItem>
            <SelectItem value="workflow">{t("workspaces:scopeWorkflow")}</SelectItem>
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">{t("workspaces:scopeKindDescription")}</p>
      </div>
      <div className="space-y-2">
        <Label>{t("workspaces:capabilities")}</Label>
        <div className="flex items-center gap-2">
          <Checkbox
            id="cap-inspect"
            checked={props.capInsp}
            onCheckedChange={(v) => props.setCapInsp(v === true)}
            data-testid="grant-cap-inspect"
          />
          <Label htmlFor="cap-inspect">inspect</Label>
        </div>
        <div className="flex items-center gap-2">
          <Checkbox
            id="cap-orchestrate"
            checked={props.capOrch}
            onCheckedChange={(v) => props.setCapOrch(v === true)}
            data-testid="grant-cap-orchestrate"
          />
          <Label htmlFor="cap-orchestrate">orchestrate</Label>
        </div>
        <p className="text-xs text-muted-foreground">{t("workspaces:capabilitiesDescription")}</p>
      </div>
      <div className="space-y-2">
        <Label htmlFor="grant-note">{t("workspaces:grantNote")}</Label>
        <Input
          id="grant-note"
          placeholder={t("workspaces:grantNotePlaceholder")}
          value={props.note}
          onChange={(e) => props.setNote(e.target.value)}
          data-testid="grant-note-input"
        />
      </div>
    </>
  );
}

export function CreateGrantDialog({ workspaceId, onCreated }: Props) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const [coordinatorTaskId, setCoordinatorTaskId] = useState("");
  const [scopeKind, setScopeKind] = useState<"workspace" | "workflow">("workspace");
  const [capInsp, setCapInsp] = useState(false);
  const [capOrch, setCapOrch] = useState(false);
  const [note, setNote] = useState("");

  const handleSubmit = async () => {
    if (!coordinatorTaskId) {
      toast.error(t("workspaces:coordinatorTaskIdRequired"));
      return;
    }
    if (!capInsp && !capOrch) {
      toast.error(t("workspaces:capabilitiesDescription"));
      return;
    }

    const caps: string[] = [];
    if (capInsp) caps.push("inspect");
    if (capOrch) caps.push("orchestrate");

    const body: CreateGrantRequest = {
      coordinator_task_id: coordinatorTaskId,
      scope_kind: scopeKind,
      capabilities: caps.join(","),
      note: note || undefined,
    };

    setSubmitting(true);
    try {
      await createWorkspaceCoordinatorGrant(workspaceId, body);
      toast.success(t("workspaces:grantCreated"));
      setOpen(false);
      setCoordinatorTaskId("");
      setScopeKind("workspace");
      setCapInsp(false);
      setCapOrch(false);
      setNote("");
      onCreated();
    } catch {
      toast.error(t("workspaces:grantCreateFailed"));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button data-testid="create-grant-button">{t("workspaces:createGrant")}</Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>{t("workspaces:createGrantTitle")}</DialogTitle>
          <DialogDescription>{t("workspaces:createGrantDescription")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <GrantFormFields
            coordinatorTaskId={coordinatorTaskId}
            setCoordinatorTaskId={setCoordinatorTaskId}
            scopeKind={scopeKind}
            setScopeKind={setScopeKind}
            capInsp={capInsp}
            setCapInsp={setCapInsp}
            capOrch={capOrch}
            setCapOrch={setCapOrch}
            note={note}
            setNote={setNote}
          />
        </div>
        <DialogFooter>
          <Button onClick={handleSubmit} disabled={submitting} data-testid="grant-create-submit">
            {t("workspaces:createGrant")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

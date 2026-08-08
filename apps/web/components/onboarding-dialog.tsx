"use client";

import { useCallback, useEffect, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import {
  IconArrowRight,
  IconArrowLeft,
  IconCheck,
  IconFolder,
  IconFolders,
  IconBrandDocker,
  IconX,
  IconLoader2,
  IconCommand,
  IconSearch,
  IconHome,
  IconGitCommit,
  IconTerminal2,
  IconArrowDown,
  IconCloud,
} from "@tabler/icons-react";
import { Kbd } from "@kandev/ui/kbd";
import { type ProfileFormData } from "@/components/settings/profile-form-fields";
import { permissionsToProfilePatch, profilePermissionValues } from "@/lib/agent-permissions";
import { listAvailableAgents, listWorkflowTemplates } from "@/lib/api";
import { listAgentsAction, updateAgentProfileAction } from "@/app/actions/agents";
import { StepAgents, type AgentSetting } from "@/components/onboarding/step-agents";
import type { AvailableAgent, ToolStatus, WorkflowTemplate, AgentProfile } from "@/lib/types/http";
import { Trans, useTranslation } from "react-i18next";

interface OnboardingDialogProps {
  open: boolean;
  onComplete: () => void;
}

const TOTAL_STEPS = 4;

// Catalog keys, not resolved copy: `t()` at module scope would freeze at the
// boot locale. `id` stays untranslated so the React key is locale-independent.
const RUNTIMES = [
  {
    id: "local",
    nameKey: "common:runtimeLocal",
    descriptionKey: "common:runtimeLocalDescription",
    icon: IconFolder,
  },
  {
    id: "worktree",
    nameKey: "common:runtimeGitWorktree",
    descriptionKey: "common:runtimeGitWorktreeDescription",
    icon: IconFolders,
  },
  {
    id: "docker",
    nameKey: "common:runtimeDocker",
    descriptionKey: "common:runtimeDockerDescription",
    icon: IconBrandDocker,
  },
  {
    id: "sprites",
    nameKey: "common:runtimeSprites",
    descriptionKey: "common:runtimeSpritesDescription",
    icon: IconCloud,
    href: "https://sprites.dev",
  },
];

const STEP_TITLE_KEYS = [
  "common:onboardingStepAgentsTitle",
  "common:onboardingStepExecutorsTitle",
  "common:onboardingStepWorkflowsTitle",
  "common:onboardingStepCommandPanelTitle",
];
const STEP_DESCRIPTION_KEYS = [
  "common:onboardingStepAgentsDescription",
  "common:onboardingStepExecutorsDescription",
  "common:onboardingStepWorkflowsDescription",
  "common:onboardingStepCommandPanelDescription",
];

function buildAgentSettings(
  avail: AvailableAgent[],
  saved: {
    name: string;
    profiles?: AgentProfile[];
  }[],
): Record<string, AgentSetting> {
  const settings: Record<string, AgentSetting> = {};
  for (const aa of avail) {
    const dbAgent = saved.find((a) => a.name === aa.name);
    const profile = dbAgent?.profiles?.[0];
    if (profile) {
      const perms = profilePermissionValues(
        {
          allowIndexing: profile.allowIndexing,
          autoApprove: profile.autoApprove,
        },
        aa.permission_settings ?? {},
      );
      settings[aa.name] = {
        profileId: profile.id,
        formData: {
          name: profile.name,
          model: profile.model || aa.model_config.default_model,
          mode: profile.mode ?? aa.model_config.current_mode_id ?? "",
          cli_passthrough: profile.cliPassthrough ?? false,
          cli_flags: profile.cliFlags ?? [],
          command_prefix: profile.commandPrefix ?? "",
          ...perms,
        },
        dirty: false,
      };
    }
  }
  return settings;
}

type OnboardingFooterProps = {
  step: number;
  onSkip: () => void;
  onBack: () => void;
  onNext: () => void;
  onGetStarted: () => void;
};

function OnboardingStepDots({ step }: { step: number }) {
  return (
    <div className="flex justify-center gap-1.5 pb-2">
      {Array.from({ length: TOTAL_STEPS }).map((_, i) => (
        <div
          key={i}
          className={`h-1.5 rounded-full transition-all ${i === step ? "w-6 bg-primary" : "w-1.5 bg-muted-foreground/30"}`}
        />
      ))}
    </div>
  );
}

function useOnboardingResources(open: boolean) {
  const [availableAgents, setAvailableAgents] = useState<AvailableAgent[]>([]);
  const [tools, setTools] = useState<ToolStatus[]>([]);
  const [agentSettings, setAgentSettings] = useState<Record<string, AgentSetting>>({});
  const [templates, setTemplates] = useState<WorkflowTemplate[]>([]);
  const [loadingAgents, setLoadingAgents] = useState(true);
  const [loadingTemplates, setLoadingTemplates] = useState(true);

  useEffect(() => {
    if (!open) return;
    queueMicrotask(() => {
      setLoadingAgents(true);
      setLoadingTemplates(true);
    });
  }, [open]);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    let timeoutId: ReturnType<typeof setTimeout> | undefined;

    // Poll while any agent is still in the "probing" state — the host-utility
    // probes async at boot and the dialog can open before they all resolve.
    // Without re-polling, agents that flip status mid-session stay stuck on
    // the initial badge in the UI. Re-poll on transient fetch errors too:
    // a single 500 shouldn't strand the dialog on stale probing status.
    let lastSawProbing = true;
    const pollOnce = (firstRun: boolean) => {
      Promise.all([
        listAvailableAgents({ cache: "no-store" }),
        firstRun ? listAgentsAction() : Promise.resolve(null),
      ])
        .then(([availRes, savedRes]) => {
          if (cancelled) return;
          const agents = availRes.agents ?? [];
          setAvailableAgents(agents);
          setTools(availRes.tools ?? []);
          if (savedRes) {
            setAgentSettings(buildAgentSettings(agents, savedRes.agents ?? []));
          }
          lastSawProbing = agents.some((a) => a.model_config.status === "probing");
        })
        .catch(() => {
          // Keep polling on transient errors — backend may be momentarily
          // unreachable while still resolving probes.
        })
        .finally(() => {
          if (cancelled) return;
          if (firstRun) setLoadingAgents(false);
          if (lastSawProbing) {
            timeoutId = setTimeout(() => pollOnce(false), 2000);
          }
        });
    };

    pollOnce(true);
    listWorkflowTemplates()
      .then((res) => {
        if (!cancelled) setTemplates(res.templates ?? []);
      })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) setLoadingTemplates(false);
      });

    return () => {
      cancelled = true;
      if (timeoutId) clearTimeout(timeoutId);
    };
  }, [open]);

  return {
    availableAgents,
    tools,
    agentSettings,
    setAgentSettings,
    templates,
    loadingAgents,
    loadingTemplates,
  };
}

function OnboardingFooter({ step, onSkip, onBack, onNext, onGetStarted }: OnboardingFooterProps) {
  const { t } = useTranslation();
  return (
    <DialogFooter>
      <div className="flex w-full items-center justify-between">
        <Button variant="ghost" size="sm" onClick={onSkip} className="cursor-pointer">
          <IconX className="mr-1.5 h-3.5 w-3.5" />
          {t("common:skip")}
        </Button>
        <div className="flex gap-2">
          {step > 0 && (
            <Button variant="outline" onClick={onBack} className="cursor-pointer">
              <IconArrowLeft className="mr-1.5 h-4 w-4" />
              {t("common:back")}
            </Button>
          )}
          {step < TOTAL_STEPS - 1 ? (
            <Button onClick={onNext} className="cursor-pointer">
              {t("common:next")}
              <IconArrowRight className="ml-1.5 h-4 w-4" />
            </Button>
          ) : (
            <Button onClick={onGetStarted} className="cursor-pointer">
              <IconCheck className="mr-1.5 h-4 w-4" />
              {t("common:getStarted")}
            </Button>
          )}
        </div>
      </div>
    </DialogFooter>
  );
}

export function OnboardingDialog({ open, onComplete }: OnboardingDialogProps) {
  const { t } = useTranslation();
  const [step, setStep] = useState(0);
  const {
    availableAgents,
    tools,
    agentSettings,
    setAgentSettings,
    templates,
    loadingAgents,
    loadingTemplates,
  } = useOnboardingResources(open);

  const saveAgentSettings = useCallback(async () => {
    await Promise.all(
      Object.values(agentSettings)
        .filter((s) => s.dirty)
        .map((s) =>
          updateAgentProfileAction(s.profileId, {
            model: s.formData.model,
            ...permissionsToProfilePatch(s.formData),
            cli_passthrough: s.formData.cli_passthrough,
            cli_flags: s.formData.cli_flags,
            command_prefix: s.formData.command_prefix,
          }),
        ),
    );
  }, [agentSettings]);

  const handleSkip = () => {
    onComplete();
    setStep(0);
  };
  const handleNext = async () => {
    if (step === 0) await saveAgentSettings();
    if (step < TOTAL_STEPS - 1) setStep(step + 1);
  };
  const handleBack = () => {
    if (step > 0) setStep(step - 1);
  };
  const handleGetStarted = async () => {
    await saveAgentSettings();
    onComplete();
    setStep(0);
  };
  const updateSetting = (agentName: string, formPatch: Partial<ProfileFormData>) => {
    setAgentSettings((prev) => ({
      ...prev,
      [agentName]: {
        ...prev[agentName],
        formData: { ...prev[agentName].formData, ...formPatch },
        dirty: true,
      },
    }));
  };

  return (
    <Dialog open={open} onOpenChange={() => {}}>
      <DialogContent className="sm:max-w-3xl" showCloseButton={false}>
        <DialogHeader>
          <DialogTitle className="text-center text-2xl">{t(STEP_TITLE_KEYS[step])}</DialogTitle>
          <DialogDescription className="text-center">
            {t(STEP_DESCRIPTION_KEYS[step])}
          </DialogDescription>
        </DialogHeader>
        <div className="py-4 min-h-[220px]">
          {step === 0 && (
            <StepAgents
              availableAgents={availableAgents}
              tools={tools}
              agentSettings={agentSettings}
              loading={loadingAgents}
              onUpdateSetting={updateSetting}
            />
          )}
          {step === 1 && <StepEnvironments />}
          {step === 2 && <StepWorkflows templates={templates} loading={loadingTemplates} />}
          {step === 3 && <StepCommandPanel />}
        </div>
        <OnboardingStepDots step={step} />
        <OnboardingFooter
          step={step}
          onSkip={handleSkip}
          onBack={handleBack}
          onNext={handleNext}
          onGetStarted={handleGetStarted}
        />
      </DialogContent>
    </Dialog>
  );
}

function StepEnvironments() {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <div className="grid gap-2">
        {RUNTIMES.map((runtime) => {
          const Icon = runtime.icon;
          const nameEl = runtime.href ? (
            <a
              href={runtime.href}
              target="_blank"
              rel="noopener noreferrer"
              className="text-sm font-medium hover:underline cursor-pointer"
            >
              {t(runtime.nameKey)}
            </a>
          ) : (
            <p className="text-sm font-medium">{t(runtime.nameKey)}</p>
          );
          return (
            <div key={runtime.id} className="flex items-start gap-3 rounded-lg border p-3">
              <div className="h-8 w-8 rounded-md bg-muted flex items-center justify-center flex-shrink-0">
                <Icon className="h-4.5 w-4.5 text-muted-foreground" />
              </div>
              <div className="min-w-0">
                {nameEl}
                <p className="text-xs text-muted-foreground">{t(runtime.descriptionKey)}</p>
              </div>
            </div>
          );
        })}
      </div>
      <p className="text-xs text-muted-foreground">
        {t("common:configureExecutorsInSettingsToControl")}
      </p>
    </div>
  );
}

function StepWorkflows({
  templates,
  loading,
}: {
  templates: WorkflowTemplate[];
  loading: boolean;
}) {
  const { t } = useTranslation();
  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-3 text-sm text-muted-foreground">
        <IconLoader2 className="h-6 w-6 animate-spin" />
        {t("common:loadingWorkflowTemplates")}
      </div>
    );
  }

  const defaultTemplate = templates.find((t) => t.id === "simple");
  const otherTemplates = templates.filter((t) => t.id !== "simple");

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-1 gap-2 max-h-[320px] overflow-y-auto">
        {defaultTemplate && <TemplateCard template={defaultTemplate} isDefault />}
        {otherTemplates.length > 0 && (
          <>
            <p className="text-xs text-muted-foreground mt-1">{t("common:availableTemplates")}</p>
            {otherTemplates.map((template) => (
              <TemplateCard key={template.id} template={template} />
            ))}
          </>
        )}
      </div>
      <p className="text-xs text-muted-foreground">
        {t("common:workflowsControlTheStepsAutomationAnd")}
      </p>
    </div>
  );
}

// `id` keeps the React key locale-independent; `labelKey` resolves at render.
const COMMAND_PANEL_PREVIEW_ITEMS = [
  { id: "search", icon: IconSearch, labelKey: "common:commandPreviewSearchTasks", trailing: "→" },
  { id: "home", icon: IconHome, labelKey: "common:commandPreviewGoToHome" },
  { id: "commit", icon: IconGitCommit, labelKey: "common:commandPreviewCommitChanges" },
  { id: "pull", icon: IconArrowDown, labelKey: "common:commandPreviewPull" },
  { id: "terminal", icon: IconTerminal2, labelKey: "common:commandPreviewAddTerminalPanel" },
];

function StepCommandPanel() {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      {/* Mock command panel preview */}
      <div className="rounded-lg border bg-card overflow-hidden">
        {/* Search input */}
        <div className="flex items-center gap-2 px-3 py-2 border-b">
          <IconSearch className="h-3.5 w-3.5 text-muted-foreground/50" />
          <span className="text-xs text-muted-foreground/50">{t("common:typeACommand")}</span>
        </div>
        {/* Sample commands */}
        <div className="py-1">
          {COMMAND_PANEL_PREVIEW_ITEMS.map((item) => {
            const Icon = item.icon;
            return (
              <div
                key={item.id}
                className="flex items-center gap-3 px-3 py-1.5 text-sm first:bg-muted/50"
              >
                <Icon className="h-3.5 w-3.5 text-muted-foreground" />
                <span className="flex-1 text-xs">{t(item.labelKey)}</span>
                {item.trailing && (
                  <span className="text-xs text-muted-foreground">{item.trailing}</span>
                )}
              </div>
            );
          })}
        </div>
        {/* Footer */}
        <div className="flex items-center gap-3 px-3 py-1.5 border-t text-muted-foreground">
          <span className="inline-flex items-center gap-1">
            <Kbd>↑</Kbd>
            <Kbd>↓</Kbd>
            <span className="text-[0.6rem]">{t("common:navigate")}</span>
          </span>
          <span className="inline-flex items-center gap-1">
            <Kbd>↵</Kbd>
            <span className="text-[0.6rem]">{t("common:select")}</span>
          </span>
          <span className="inline-flex items-center gap-1">
            {/* A key name, not copy — it labels the physical key. */}
            <Kbd>esc</Kbd>
            <span className="text-[0.6rem]">{t("common:close")}</span>
          </span>
        </div>
      </div>

      {/* Shortcut hint */}
      {/* One message with indexed tags rather than a "Press" stem and a tail:
          the flex children keep their gap-2 spacing, and a translator can still
          move the key chord to wherever the sentence needs it. */}
      <div className="flex items-center justify-center gap-2 text-sm text-muted-foreground">
        <Trans i18nKey="common:pressCmdKToOpenItAnytime">
          <span>Press</span>
          <span className="inline-flex items-center gap-0.5">
            <Kbd>
              <IconCommand className="size-3" />
            </Kbd>
            <Kbd>K</Kbd>
          </span>
          <span>to open it anytime</span>
        </Trans>
      </div>

      <p className="text-xs text-muted-foreground">
        {t("common:navigateBetweenPagesSearchTasksTrigger")}
      </p>
    </div>
  );
}

function TemplateCard({
  template,
  isDefault,
}: {
  template: WorkflowTemplate;
  isDefault?: boolean;
}) {
  const { t } = useTranslation();
  const steps = (template.default_steps ?? []).slice().sort((a, b) => a.position - b.position);

  return (
    <div
      className={`rounded-lg border p-3 ${isDefault ? "border-primary/50 bg-primary/5" : "opacity-60"}`}
    >
      <div className="flex items-center gap-2">
        <p className="text-sm font-medium">{template.name}</p>
        {isDefault && (
          <span className="flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
            <IconCheck className="h-3.5 w-3.5" />
            {t("common:defaultBadge")}
          </span>
        )}
      </div>
      {template.description && (
        <p className="text-xs text-muted-foreground mt-0.5">{template.description}</p>
      )}
      {steps.length > 0 && (
        <div className="flex flex-wrap items-center gap-x-1.5 gap-y-1 text-xs text-muted-foreground mt-2">
          {steps.map((s, i) => (
            <span key={s.name} className="flex items-center gap-1.5">
              {i > 0 && <span className="text-muted-foreground/40">→</span>}
              <span className="flex items-center gap-1">
                <span
                  className="h-1.5 w-1.5 rounded-full shrink-0"
                  style={{ backgroundColor: s.color || "hsl(var(--muted-foreground))" }}
                />
                {s.name}
              </span>
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

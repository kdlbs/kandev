'use client';

import { Fragment, useCallback, useEffect, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from '@kandev/ui/dialog';
import { Button } from '@kandev/ui/button';
import {
  IconArrowRight,
  IconArrowLeft,
  IconCheck,
  IconFolder,
  IconFolders,
  IconBrandDocker,
  IconX,
  IconLoader2,
  IconChevronDown,
} from '@tabler/icons-react';
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from '@kandev/ui/collapsible';
import { Switch } from '@kandev/ui/switch';
import { Label } from '@kandev/ui/label';
import { AgentLogo } from '@/components/agent-logo';
import { ModelCombobox } from '@/components/settings/model-combobox';
import { listAvailableAgents, listWorkflowTemplates } from '@/lib/api';
import { listAgentsAction, updateAgentProfileAction } from '@/app/actions/agents';
import type { AvailableAgent, WorkflowTemplate } from '@/lib/types/http';

interface OnboardingDialogProps {
  open: boolean;
  onComplete: () => void;
}

type AgentSetting = {
  profileId: string;
  model: string;
  permissions: Record<string, boolean>;
  passthrough: boolean;
  dirty: boolean;
};

const TOTAL_STEPS = 3;

const RUNTIMES = [
  {
    name: 'Local',
    description: 'Run agents directly on your machine with full access to your local filesystem.',
    icon: IconFolder,
  },
  {
    name: 'Git Worktree',
    description: 'Isolated branch environment under a worktree root for parallel work.',
    icon: IconFolders,
  },
  {
    name: 'Docker',
    description: 'Containerized execution for full isolation and reproducibility.',
    icon: IconBrandDocker,
  },
];

export function OnboardingDialog({ open, onComplete }: OnboardingDialogProps) {
  const [step, setStep] = useState(0);
  const [availableAgents, setAvailableAgents] = useState<AvailableAgent[]>([]);
  const [agentSettings, setAgentSettings] = useState<Record<string, AgentSetting>>({});
  const [templates, setTemplates] = useState<WorkflowTemplate[]>([]);
  const [loadingAgents, setLoadingAgents] = useState(true);
  const [loadingTemplates, setLoadingTemplates] = useState(true);
  const [prevOpen, setPrevOpen] = useState(false);

  // Reset loading states during render when dialog opens (avoids setState in effect)
  if (open && !prevOpen) {
    setPrevOpen(true);
    setLoadingAgents(true);
    setLoadingTemplates(true);
  } else if (!open && prevOpen) {
    setPrevOpen(false);
  }

  useEffect(() => {
    if (!open) return;

    Promise.all([
      listAvailableAgents({ cache: 'no-store' }),
      listAgentsAction(),
    ])
      .then(([availRes, savedRes]) => {
        const avail = availRes.agents ?? [];
        const saved = savedRes.agents ?? [];
        setAvailableAgents(avail);

        // Build initial agentSettings by matching agent names
        const settings: Record<string, AgentSetting> = {};
        for (const aa of avail) {
          const dbAgent = saved.find((a) => a.name === aa.name);
          const profile = dbAgent?.profiles?.[0];
          if (profile) {
            const perms: Record<string, boolean> = {};
            if (aa.permission_settings) {
              for (const [key, setting] of Object.entries(aa.permission_settings)) {
                if (setting.supported) {
                  // Map permission key to profile field
                  const profileValue = getProfilePermissionValue(profile, key);
                  perms[key] = profileValue ?? setting.default;
                }
              }
            }
            settings[aa.name] = {
              profileId: profile.id,
              model: profile.model || aa.model_config.default_model,
              permissions: perms,
              passthrough: profile.cli_passthrough ?? false,
              dirty: false,
            };
          }
        }
        setAgentSettings(settings);
      })
      .catch(() => { })
      .finally(() => setLoadingAgents(false));

    listWorkflowTemplates()
      .then((res) => setTemplates(res.templates ?? []))
      .catch(() => { })
      .finally(() => setLoadingTemplates(false));
  }, [open]);

  const saveAgentSettings = useCallback(async () => {
    const dirtyEntries = Object.entries(agentSettings).filter(([, s]) => s.dirty);
    await Promise.all(
      dirtyEntries.map(([, s]) =>
        updateAgentProfileAction(s.profileId, {
          model: s.model,
          auto_approve: s.permissions['auto_approve'] ?? false,
          dangerously_skip_permissions: s.permissions['dangerously_skip_permissions'] ?? false,
          allow_indexing: s.permissions['allow_indexing'] ?? false,
          cli_passthrough: s.passthrough,
        })
      )
    );
  }, [agentSettings]);

  const handleSkip = () => {
    onComplete();
    setStep(0);
  };

  const handleNext = async () => {
    if (step === 0) {
      await saveAgentSettings();
    }
    if (step < TOTAL_STEPS - 1) {
      setStep(step + 1);
    }
  };

  const handleBack = () => {
    if (step > 0) {
      setStep(step - 1);
    }
  };

  const handleGetStarted = async () => {
    await saveAgentSettings();
    onComplete();
    setStep(0);
  };

  const updateSetting = (agentName: string, patch: Partial<AgentSetting>) => {
    setAgentSettings((prev) => ({
      ...prev,
      [agentName]: {
        ...prev[agentName],
        ...patch,
        dirty: true,
      },
    }));
  };

  return (
    <Dialog open={open} onOpenChange={() => { }}>
      <DialogContent className="sm:max-w-[540px]" showCloseButton={false}>
        <DialogHeader>
          <DialogTitle className="text-center text-2xl">
            {step === 0 && 'AI Agents'}
            {step === 1 && 'Environments'}
            {step === 2 && 'Agentic Workflows'}
          </DialogTitle>
          <DialogDescription className="text-center">
            {step === 0 && 'These AI coding agents were discovered on your system.'}
            {step === 1 && 'Agents can run in different runtime environments.'}
            {step === 2 && 'Workflows define the steps and automation for your tasks.'}
          </DialogDescription>
        </DialogHeader>

        <div className="py-4 min-h-[220px]">
          {step === 0 && (
            <StepAgents
              availableAgents={availableAgents}
              agentSettings={agentSettings}
              loading={loadingAgents}
              onUpdateSetting={updateSetting}
            />
          )}
          {step === 1 && <StepEnvironments />}
          {step === 2 && <StepWorkflows templates={templates} loading={loadingTemplates} />}
        </div>

        {/* Progress dots */}
        <div className="flex justify-center gap-1.5 pb-2">
          {Array.from({ length: TOTAL_STEPS }).map((_, i) => (
            <div
              key={i}
              className={`h-1.5 rounded-full transition-all ${i === step ? 'w-6 bg-primary' : 'w-1.5 bg-muted-foreground/30'
                }`}
            />
          ))}
        </div>

        <DialogFooter>
          <div className="flex w-full items-center justify-between">
            <Button variant="ghost" size="sm" onClick={handleSkip} className="cursor-pointer">
              <IconX className="mr-1.5 h-3.5 w-3.5" />
              Skip
            </Button>
            <div className="flex gap-2">
              {step > 0 && (
                <Button variant="outline" onClick={handleBack} className="cursor-pointer">
                  <IconArrowLeft className="mr-1.5 h-4 w-4" />
                  Back
                </Button>
              )}
              {step < TOTAL_STEPS - 1 ? (
                <Button onClick={handleNext} className="cursor-pointer">
                  Next
                  <IconArrowRight className="ml-1.5 h-4 w-4" />
                </Button>
              ) : (
                <Button onClick={handleGetStarted} className="cursor-pointer">
                  <IconCheck className="mr-1.5 h-4 w-4" />
                  Get Started
                </Button>
              )}
            </div>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** Map a permission settings key to the corresponding AgentProfile boolean field value. */
function getProfilePermissionValue(
  profile: { auto_approve?: boolean; dangerously_skip_permissions?: boolean; allow_indexing?: boolean },
  key: string
): boolean | undefined {
  switch (key) {
    case 'auto_approve':
      return profile.auto_approve;
    case 'dangerously_skip_permissions':
      return profile.dangerously_skip_permissions;
    case 'allow_indexing':
      return profile.allow_indexing;
    default:
      return undefined;
  }
}

function StepAgents({
  availableAgents,
  agentSettings,
  loading,
  onUpdateSetting,
}: {
  availableAgents: AvailableAgent[];
  agentSettings: Record<string, AgentSetting>;
  loading: boolean;
  onUpdateSetting: (agentName: string, patch: Partial<AgentSetting>) => void;
}) {
  const [openAgent, setOpenAgent] = useState<string | null>(null);

  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-3 text-sm text-muted-foreground">
        <IconLoader2 className="h-6 w-6 animate-spin" />
        Discovering agents...
      </div>
    );
  }

  const agents = availableAgents.filter((a) => a.available);

  return (
    <div className="space-y-3">
      <div className="grid gap-2 max-h-[320px] overflow-y-auto pr-1">
        {agents.map((agent) => {
          const settings = agentSettings[agent.name];
          const modelName =
            agent.model_config.available_models.find(
              (m) => m.id === (settings?.model || agent.model_config.default_model)
            )?.name ?? settings?.model ?? agent.model_config.default_model;

          return (
            <Collapsible
              key={agent.name}
              open={openAgent === agent.name}
              onOpenChange={(isOpen) => setOpenAgent(isOpen ? agent.name : null)}
            >
              <CollapsibleTrigger asChild>
                <button
                  type="button"
                  className="flex w-full items-center gap-3 rounded-lg border p-3 text-left cursor-pointer hover:bg-muted/50 transition-colors group"
                >
                  <AgentLogo agentName={agent.name} size={28} />
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium">{agent.display_name}</p>
                  </div>
                  <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground font-medium truncate max-w-[120px]">
                    {modelName}
                  </span>
                  <span className="flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
                    <IconCheck className="h-3.5 w-3.5" />
                    Installed
                  </span>
                  <IconChevronDown className="h-4 w-4 text-muted-foreground transition-transform group-data-[state=open]:rotate-180" />
                </button>
              </CollapsibleTrigger>
              <CollapsibleContent>
                <div className="border border-t-0 rounded-b-lg px-3 pb-3 pt-2 space-y-3">
                  {/* Model selector */}
                  <div className="space-y-1.5">
                    <Label className="text-xs text-muted-foreground">Model</Label>
                    <ModelCombobox
                      value={settings?.model || agent.model_config.default_model}
                      onChange={(value) => onUpdateSetting(agent.name, { model: value })}
                      models={agent.model_config.available_models}
                      defaultModel={agent.model_config.default_model}
                      agentName={agent.name}
                      supportsDynamicModels={agent.model_config.supports_dynamic_models}
                    />
                  </div>

                  {/* Permission toggles */}
                  {agent.permission_settings &&
                    Object.entries(agent.permission_settings).map(([key, perm]) => {
                      if (!perm.supported) return null;
                      return (
                        <div key={key} className="flex items-center justify-between gap-2">
                          <div className="space-y-0.5">
                            <Label className="text-xs">{perm.label}</Label>
                            <p className="text-[10px] text-muted-foreground leading-tight">
                              {perm.description}
                            </p>
                          </div>
                          <Switch
                            size="sm"
                            checked={settings?.permissions[key] ?? perm.default}
                            onCheckedChange={(checked) =>
                              onUpdateSetting(agent.name, {
                                permissions: {
                                  ...settings?.permissions,
                                  [key]: checked === true,
                                },
                              })
                            }
                          />
                        </div>
                      );
                    })}

                  {/* Passthrough toggle */}
                  {agent.passthrough_config?.supported && (
                    <div className="flex items-center justify-between gap-2">
                      <div className="space-y-0.5">
                        <Label className="text-xs">{agent.passthrough_config.label}</Label>
                        <p className="text-[10px] text-muted-foreground leading-tight">
                          {agent.passthrough_config.description}
                        </p>
                      </div>
                      <Switch
                        size="sm"
                        checked={settings?.passthrough ?? false}
                        onCheckedChange={(checked) =>
                          onUpdateSetting(agent.name, { passthrough: checked === true })
                        }
                      />
                    </div>
                  )}
                </div>
              </CollapsibleContent>
            </Collapsible>
          );
        })}
      </div>
      <p className="text-xs text-muted-foreground">
        Expand an agent to configure its model and permissions. Changes are saved when you proceed.
      </p>
    </div>
  );
}

function StepEnvironments() {
  return (
    <div className="space-y-3">
      <div className="grid gap-2">
        {RUNTIMES.map((runtime) => {
          const Icon = runtime.icon;
          return (
            <div
              key={runtime.name}
              className="flex items-start gap-3 rounded-lg border p-3"
            >
              <div className="h-8 w-8 rounded-md bg-muted flex items-center justify-center flex-shrink-0">
                <Icon className="h-4.5 w-4.5 text-muted-foreground" />
              </div>
              <div className="min-w-0">
                <p className="text-sm font-medium">{runtime.name}</p>
                <p className="text-xs text-muted-foreground">{runtime.description}</p>
              </div>
            </div>
          );
        })}
      </div>
      <p className="text-xs text-muted-foreground">
        Configure runtime environments in Settings to control where agents execute.
      </p>
    </div>
  );
}

function StepWorkflows({ templates, loading }: { templates: WorkflowTemplate[]; loading: boolean }) {
  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-3 text-sm text-muted-foreground">
        <IconLoader2 className="h-6 w-6 animate-spin" />
        Loading workflow templates...
      </div>
    );
  }

  const defaultTemplate = templates.find((t) => t.id === 'simple');
  const otherTemplates = templates.filter((t) => t.id !== 'simple');

  return (
    <div className="space-y-3">
      <div className="grid gap-2 max-h-[320px] overflow-y-auto pr-1">
        {defaultTemplate && (
          <TemplateCard template={defaultTemplate} isDefault />
        )}
        {otherTemplates.length > 0 && (
          <>
            <p className="text-xs text-muted-foreground mt-1">Available templates</p>
            {otherTemplates.map((template) => (
              <TemplateCard key={template.id} template={template} />
            ))}
          </>
        )}
      </div>
      <p className="text-xs text-muted-foreground">
        Workflows control the steps, automation, and agent behavior for your tasks. You can add
        more workflows from Settings.
      </p>
    </div>
  );
}

function TemplateCard({ template, isDefault }: { template: WorkflowTemplate; isDefault?: boolean }) {
  const steps = (template.default_steps ?? [])
    .slice()
    .sort((a, b) => a.position - b.position);

  return (
    <div
      className={`rounded-lg border p-3 ${isDefault ? 'border-primary/50 bg-primary/5' : 'opacity-60'}`}
    >
      <div className="flex items-center gap-2">
        <p className="text-sm font-medium">{template.name}</p>
        {isDefault && (
          <span className="flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
            <IconCheck className="h-3.5 w-3.5" />
            Default
          </span>
        )}
      </div>
      {template.description && (
        <p className="text-xs text-muted-foreground mt-0.5">{template.description}</p>
      )}
      {steps.length > 0 && (
        <div className="flex items-center gap-1.5 text-xs text-muted-foreground whitespace-nowrap mt-2">
          {steps.map((s, i) => (
            <Fragment key={s.name}>
              {i > 0 && <span className="text-muted-foreground/40">→</span>}
              <span className="flex items-center gap-1">
                <span
                  className="h-1.5 w-1.5 rounded-full shrink-0"
                  style={{ backgroundColor: s.color || 'hsl(var(--muted-foreground))' }}
                />
                {s.name}
              </span>
            </Fragment>
          ))}
        </div>
      )}
    </div>
  );
}

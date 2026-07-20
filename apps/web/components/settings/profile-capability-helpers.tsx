import { IconTerminal2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@kandev/ui/dialog";
import type { CommandEntry, ModeEntry } from "@/lib/types/http";
import type { ProfileFormData } from "./profile-form-fields";

export function CommandsButton({ commands }: { commands: CommandEntry[] }) {
  if (commands.length === 0) return null;
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="cursor-pointer"
          data-testid="profile-commands-button"
        >
          <IconTerminal2 className="mr-2 h-4 w-4" />
          Available commands ({commands.length})
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Available slash commands</DialogTitle>
          <DialogDescription>
            Type these during a session chat to invoke them - e.g. <code>/init</code>.
          </DialogDescription>
        </DialogHeader>
        <div className="max-h-[60vh] overflow-y-auto space-y-2">
          {commands.map((command) => (
            <div key={command.name} className="rounded-md border p-3">
              <code className="text-sm font-semibold">/{command.name}</code>
              {command.description && (
                <p className="text-xs text-muted-foreground mt-1">{command.description}</p>
              )}
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}

export function findActiveMode(
  modes: ModeEntry[],
  selectedMode: string,
  currentModeId?: string,
): ModeEntry | undefined {
  if (modes.length === 0) return undefined;
  const modeId = selectedMode || currentModeId || modes[0]?.id;
  return modes.find((mode) => mode.id === modeId);
}

export function profileModelIsDirty(profile: ProfileFormData, baseline?: ProfileFormData): boolean {
  if (!baseline) return false;
  return (
    profile.model !== baseline.model ||
    JSON.stringify(profile.config_options ?? {}) !== JSON.stringify(baseline.config_options ?? {})
  );
}

export function profileModeIsDirty(profile: ProfileFormData, baseline?: ProfileFormData): boolean {
  return Boolean(baseline && profile.mode !== baseline.mode);
}

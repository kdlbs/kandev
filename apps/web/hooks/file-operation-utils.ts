import type { useToast } from "@/components/toast-provider";

type ToastFn = ReturnType<typeof useToast>["toast"];
const ERROR_VARIANT = "error" as const;

export async function runFileOp<T extends { success: boolean; error?: string }>(
  op: () => Promise<T>,
  title: string,
  unknownError: string,
  toast: ToastFn,
): Promise<boolean> {
  try {
    const response = await op();
    if (!response.success) {
      toast({ title, description: response.error || unknownError, variant: ERROR_VARIANT });
      return false;
    }
    return true;
  } catch (error) {
    const description = error instanceof Error ? error.message : unknownError;
    toast({ title, description, variant: ERROR_VARIANT });
    return false;
  }
}

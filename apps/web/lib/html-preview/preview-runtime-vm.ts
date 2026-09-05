import { getQuickJS } from "quickjs-emscripten";
import type {
  QuickJSContext,
  QuickJSHandle,
  QuickJSRuntime,
  QuickJSWASMModule,
} from "quickjs-emscripten";
import { PreviewVirtualDocument } from "./preview-runtime-document";
import {
  PreviewRuntimeError,
  type PreviewEvent,
  type PreviewRuntimeOptions,
  type PreviewSnapshot,
} from "./preview-runtime-types";
import { PreviewRuntimeVmHost } from "./preview-runtime-vm-host";

export const DEFAULT_PREVIEW_RUNTIME_OPTIONS: PreviewRuntimeOptions = {
  instructionBudget: 250_000,
  wallClockBudgetMs: 250,
  memoryLimitBytes: 8 * 1024 * 1024,
  maxStackSizeBytes: 512 * 1024,
  maxTimers: 16,
  maxEventQueue: 32,
  maxSnapshotBytes: 512 * 1024,
};

const QUICKJS_INTERRUPT_INSTRUCTION_INTERVAL = 10_000;

export class PreviewRuntimeVm {
  private readonly options: PreviewRuntimeOptions;
  private readonly quickJS: QuickJSWASMModule;
  private readonly ownedHandles: QuickJSHandle[] = [];
  private runtime: QuickJSRuntime | undefined;
  private context: QuickJSContext | undefined;
  private host: PreviewRuntimeVmHost | undefined;
  private startedAt = 0;
  private instructionChecks = 0;
  private budgetExceeded = false;
  private disposed = false;

  private constructor(quickJS: QuickJSWASMModule, options: PreviewRuntimeOptions) {
    this.quickJS = quickJS;
    this.options = { ...DEFAULT_PREVIEW_RUNTIME_OPTIONS, ...options };
  }

  static async create(options: PreviewRuntimeOptions): Promise<PreviewRuntimeVm> {
    try {
      return new PreviewRuntimeVm(await getQuickJS(), options);
    } catch {
      throw new PreviewRuntimeError("initialization-failed");
    }
  }

  async load(source: string): Promise<PreviewSnapshot> {
    this.ensureUsable();
    this.disposeExecution();
    this.startedAt = Date.now();
    this.instructionChecks = 0;
    this.budgetExceeded = false;
    try {
      const document = new PreviewVirtualDocument(source);
      this.runtime = this.quickJS.newRuntime();
      this.runtime.setMemoryLimit(this.options.memoryLimitBytes);
      this.runtime.setMaxStackSize(this.options.maxStackSizeBytes);
      this.runtime.setInterruptHandler(() => {
        this.instructionChecks += 1;
        if (Date.now() - this.startedAt > this.options.wallClockBudgetMs) {
          this.budgetExceeded = true;
          return true;
        }
        if (
          this.instructionChecks * QUICKJS_INTERRUPT_INSTRUCTION_INTERVAL >
          this.options.instructionBudget
        ) {
          this.budgetExceeded = true;
          return true;
        }
        return false;
      });
      this.context = this.runtime.newContext();
      this.host = new PreviewRuntimeVmHost({
        runtime: this.runtime,
        context: this.context,
        document,
        options: this.options,
        own: (handle) => this.own(handle),
        releaseHandle: (handle) => this.releaseHandle(handle),
        isBudgetExceeded: () => this.budgetExceeded,
      });
      this.host.start(this.startedAt);
      this.host.runScripts();
      return this.host.snapshot();
    } catch (error) {
      throw this.asRuntimeError(error);
    }
  }

  async dispatch(event: PreviewEvent): Promise<PreviewSnapshot> {
    this.ensureUsable();
    if (!this.host) throw new PreviewRuntimeError("runtime-error");
    this.startedAt = Date.now();
    this.instructionChecks = 0;
    this.budgetExceeded = false;
    this.host.beginOperation(this.startedAt);
    try {
      return this.host.dispatch(event);
    } catch (error) {
      throw this.asRuntimeError(error);
    }
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.disposeExecution();
  }

  private disposeExecution(): void {
    this.host?.dispose();
    this.host = undefined;
    for (const handle of [...this.ownedHandles].reverse()) {
      try {
        if (handle.alive) handle.dispose();
      } catch {
        // Context disposal below remains the final ownership boundary.
      }
    }
    this.ownedHandles.length = 0;
    this.runtime?.removeInterruptHandler();
    try {
      this.context?.dispose();
    } catch {
      // A failed preview must not prevent the source view from being restored.
    }
    try {
      this.runtime?.dispose();
    } catch {
      // The worker is disposable even when QuickJS reports a late cleanup error.
    }
    this.context = undefined;
    this.runtime = undefined;
  }

  private ensureUsable(): void {
    if (this.disposed) throw new PreviewRuntimeError("disposed");
  }

  private asRuntimeError(error: unknown): PreviewRuntimeError {
    if (error instanceof PreviewRuntimeError) return error;
    if (
      this.budgetExceeded ||
      /preview budget exceeded|budget-exceeded/i.test(this.errorMessage(error))
    ) {
      return new PreviewRuntimeError("budget-exceeded");
    }
    if (/unsupported-capability|not supported/i.test(this.errorMessage(error))) {
      return new PreviewRuntimeError("unsupported-capability");
    }
    return new PreviewRuntimeError("runtime-error");
  }

  private errorMessage(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }

  private own<T extends QuickJSHandle>(handle: T): T {
    this.ownedHandles.push(handle);
    return handle;
  }

  private releaseHandle(handle: QuickJSHandle): void {
    const index = this.ownedHandles.indexOf(handle);
    if (index >= 0) this.ownedHandles.splice(index, 1);
    try {
      if (handle.alive) handle.dispose();
    } catch {
      // The owning context performs final cleanup.
    }
  }
}

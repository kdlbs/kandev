import type { StoreApi } from 'zustand';
import type { AppState } from '@/lib/state/store';
import type { WsHandlers } from '@/lib/ws/handlers/types';

export function registerTerminalsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    'terminal.output': (message) => {
      store.getState().setTerminalOutput(message.payload.terminalId, message.payload.data);
    },
    'session.shell.output': (message) => {
      const { session_id, type, data } = message.payload;
      if (!session_id) {
        return;
      }
      if (type === 'output' && data) {
        store.getState().appendShellOutput(session_id, data);
      } else if (type === 'exit') {
        // Shell exited - update status
        store.getState().setShellStatus(session_id, { available: false });
      }
    },
    'session.process.output': (message) => {
      const { process_id, session_id, kind, data } = message.payload;
      if (!process_id || !data) {
        return;
      }
      store.getState().appendProcessOutput(process_id, data);
      // For passthrough mode, also store output under session_id for the PassthroughTerminal
      if (kind === 'agent_passthrough' && session_id) {
        store.getState().appendShellOutput(`passthrough:${session_id}`, data);
      }
    },
    'session.process.status': (message) => {
      const {
        session_id,
        process_id,
        kind,
        status,
        script_name,
        command,
        working_dir,
        exit_code,
        timestamp,
      } = message.payload;
      if (!session_id || !process_id || !status) {
        return;
      }
      store.getState().upsertProcessStatus({
        processId: process_id,
        sessionId: session_id,
        kind,
        scriptName: script_name,
        status,
        command,
        workingDir: working_dir,
        exitCode: exit_code,
        updatedAt: timestamp,
      });
      if (status === 'starting') {
        store.getState().clearProcessOutput(process_id);
      }
    },
  };
}

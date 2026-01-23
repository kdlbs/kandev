'use client';

import { useEffect, useRef } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';

const DEFAULT_FONT = 'Menlo, Monaco, "Courier New", monospace';

type ProcessOutputTerminalProps = {
  output: string;
  processId?: string | null;
  isStopping?: boolean;
};

export function ProcessOutputTerminal({ output, processId, isStopping = false }: ProcessOutputTerminalProps) {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const lastOutputLengthRef = useRef(0);
  const processIdRef = useRef<string | null>(null);
  const outputRef = useRef(output);

  useEffect(() => {
    outputRef.current = output;
  }, [output]);

  useEffect(() => {
    if (!terminalRef.current || xtermRef.current) return;

    const terminal = new Terminal({
      cursorBlink: false,
      disableStdin: true,
      convertEol: true,
      fontSize: 12,
      fontFamily: DEFAULT_FONT,
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
        cursor: '#d4d4d4',
        cursorAccent: '#1e1e1e',
        selectionBackground: '#264f78',
        black: '#1e1e1e',
        red: '#f44747',
        green: '#6a9955',
        yellow: '#dcdcaa',
        blue: '#569cd6',
        magenta: '#c586c0',
        cyan: '#4ec9b0',
        white: '#d4d4d4',
        brightBlack: '#808080',
        brightRed: '#f44747',
        brightGreen: '#6a9955',
        brightYellow: '#dcdcaa',
        brightBlue: '#569cd6',
        brightMagenta: '#c586c0',
        brightCyan: '#4ec9b0',
        brightWhite: '#ffffff',
      },
    });

    const fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);
    terminal.open(terminalRef.current);
    fitAddon.fit();

    xtermRef.current = terminal;
    fitAddonRef.current = fitAddon;

    if (outputRef.current) {
      terminal.write(outputRef.current);
      lastOutputLengthRef.current = outputRef.current.length;
    }

    const initialFitTimeout = setTimeout(() => {
      fitAddon.fit();
    }, 100);

    const resizeObserver = new ResizeObserver(() => {
      fitAddon.fit();
    });
    resizeObserver.observe(terminalRef.current);

    const intersectionObserver = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (entry.isIntersecting && fitAddonRef.current) {
            requestAnimationFrame(() => {
              fitAddonRef.current?.fit();
            });
          }
        });
      },
      { threshold: 0.1 }
    );
    intersectionObserver.observe(terminalRef.current);

    return () => {
      clearTimeout(initialFitTimeout);
      resizeObserver.disconnect();
      intersectionObserver.disconnect();
      terminal.dispose();
      xtermRef.current = null;
      fitAddonRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (!xtermRef.current) return;
    if (processIdRef.current !== processId) {
      processIdRef.current = processId ?? null;
      lastOutputLengthRef.current = 0;
      xtermRef.current.clear();
      if (outputRef.current) {
        xtermRef.current.write(outputRef.current);
        lastOutputLengthRef.current = outputRef.current.length;
      }
    }
  }, [processId]);

  useEffect(() => {
    if (!xtermRef.current) return;
    const newData = output.slice(lastOutputLengthRef.current);
    if (newData) {
      xtermRef.current.write(newData);
      lastOutputLengthRef.current = output.length;
    }
  }, [output]);

  return (
    <div className="h-full w-full rounded-md bg-[#1e1e1e] relative">
      <div ref={terminalRef} className="absolute inset-0" />
      {isStopping ? (
        <div className="absolute right-3 top-2 text-xs text-muted-foreground">
          Stopping…
        </div>
      ) : null}
    </div>
  );
}

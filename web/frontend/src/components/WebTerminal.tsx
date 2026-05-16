import "@xterm/xterm/css/xterm.css";
import { useEffect, useRef, useState } from "react";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";

type TerminalStatus = "idle" | "connecting" | "connected" | "closed" | "error";

type TerminalMessage = {
  type: "ready" | "output" | "exit" | "error";
  data?: string;
  error?: string;
  cwd?: string;
  shell?: string;
  exitCode?: number;
};

type WebTerminalProps = {
  cwd: string;
  shell: string;
  endpoint: string;
};

export function WebTerminal({ cwd, shell, endpoint }: WebTerminalProps) {
  const mountRef = useRef<HTMLDivElement | null>(null);
  const termRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const socketRef = useRef<WebSocket | null>(null);
  const resizeRef = useRef<ResizeObserver | null>(null);
  const [status, setStatus] = useState<TerminalStatus>("idle");
  const [sessionLabel, setSessionLabel] = useState("terminal idle");

  function writeLine(text: string) {
    termRef.current?.writeln(text);
  }

  function sendResize() {
    const socket = socketRef.current;
    const fit = fitRef.current;
    if (!socket || socket.readyState !== WebSocket.OPEN || !fit) return;

    const nextSize = fit.proposeDimensions();
    if (!nextSize) return;

    socket.send(
      JSON.stringify({
        type: "resize",
        cols: nextSize.cols,
        rows: nextSize.rows
      })
    );
  }

  function disconnectTerminal() {
    socketRef.current?.close();
    socketRef.current = null;
    setStatus("closed");
  }

  function startTerminal() {
    disconnectTerminal();

    const term = termRef.current;
    const fit = fitRef.current;
    if (!term || !fit) return;

    term.clear();
    term.writeln(`connecting ${shell} in ${cwd}...`);
    term.focus();
    setStatus("connecting");

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsURL = new URL(endpoint, `${protocol}//${window.location.host}`);
    wsURL.searchParams.set("cwd", cwd);
    wsURL.searchParams.set("shell", shell);

    const size = fit.proposeDimensions();
    if (size) {
      wsURL.searchParams.set("cols", String(size.cols));
      wsURL.searchParams.set("rows", String(size.rows));
    }

    const socket = new WebSocket(wsURL);
    socketRef.current = socket;

    socket.onopen = () => {
      setStatus("connected");
      sendResize();
    };

    socket.onmessage = (event) => {
      const payload = JSON.parse(event.data) as TerminalMessage;
      if (payload.type === "ready") {
        setSessionLabel(`${payload.shell ?? shell} @ ${payload.cwd ?? cwd}`);
        term.writeln("");
        term.writeln(`[connected] ${payload.shell ?? shell} @ ${payload.cwd ?? cwd}`);
        term.focus();
        return;
      }

      if (payload.type === "output" && payload.data) {
        term.write(payload.data);
        return;
      }

      if (payload.type === "error") {
        setStatus("error");
        term.writeln("");
        term.writeln(`[error] ${payload.error ?? payload.data ?? "terminal error"}`);
        return;
      }

      if (payload.type === "exit") {
        setStatus("closed");
        term.writeln("");
        term.writeln(`[exit ${payload.exitCode ?? 0}]`);
      }
    };

    socket.onerror = () => {
      setStatus("error");
      writeLine("");
      writeLine("[error] terminal socket failed");
    };

    socket.onclose = () => {
      if (socketRef.current === socket) {
        socketRef.current = null;
      }
      setStatus((current) => (current === "error" ? "error" : "closed"));
    };
  }

  useEffect(() => {
    const mount = mountRef.current;
    if (!mount) return;

    const term = new Terminal({
      cursorBlink: true,
      fontFamily: '"JetBrains Mono", "SFMono-Regular", ui-monospace, monospace',
      fontSize: 13,
      lineHeight: 1.35,
      scrollback: 3000,
      theme: {
        background: "#1c1510",
        foreground: "#f2e5d5",
        cursor: "#fdf3e7",
        black: "#1c1510",
        red: "#d5674f",
        green: "#78b284",
        yellow: "#d0a65f",
        blue: "#77a4d2",
        magenta: "#b688d7",
        cyan: "#7ec5ca",
        white: "#f2e5d5",
        brightBlack: "#66584c",
        brightRed: "#ef8667",
        brightGreen: "#95d09b",
        brightYellow: "#efc576",
        brightBlue: "#93bde4",
        brightMagenta: "#d4a8ef",
        brightCyan: "#a1e0e2",
        brightWhite: "#fff7ef"
      }
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(mount);
    fit.fit();
    term.focus();
    term.writeln("StoryForge PTY terminal");
    term.writeln("Click start to open an interactive shell.");

    term.onData((data) => {
      const socket = socketRef.current;
      if (!socket || socket.readyState !== WebSocket.OPEN) return;
      socket.send(JSON.stringify({ type: "input", data }));
    });

    const resizeObserver = new ResizeObserver(() => {
      fit.fit();
      sendResize();
    });
    resizeObserver.observe(mount);
    const focusTerminal = () => term.focus();
    mount.addEventListener("click", focusTerminal);

    termRef.current = term;
    fitRef.current = fit;
    resizeRef.current = resizeObserver;

    return () => {
      resizeObserver.disconnect();
      resizeRef.current = null;
      mount.removeEventListener("click", focusTerminal);
      socketRef.current?.close();
      socketRef.current = null;
      fitRef.current = null;
      termRef.current?.dispose();
      termRef.current = null;
    };
  }, []);

  useEffect(() => {
    setSessionLabel(`${shell} @ ${cwd}`);
  }, [cwd, shell]);

  return (
    <div className="terminalPanel">
      <div className="terminalMeta">
        <span className={`signalBadge ${status === "connected" ? "streaming" : status === "connecting" ? "connecting" : status}`}>
          {status}
        </span>
        <span>{sessionLabel}</span>
      </div>

      <div className="toolbar">
        <button className="primaryButton" onClick={startTerminal} type="button">
          {status === "connected" ? "重连终端" : "启动终端"}
        </button>
        <button onClick={disconnectTerminal} disabled={status !== "connected" && status !== "connecting"} type="button">
          断开
        </button>
        <button
          onClick={() => {
            termRef.current?.clear();
            writeLine("terminal cleared");
          }}
          type="button"
        >
          清屏
        </button>
      </div>

      <div className="terminalSurface">
        <div className="terminalHeader">
          <strong>{sessionLabel}</strong>
          <span>{endpoint}</span>
        </div>
        <div className="terminalMount" ref={mountRef} />
      </div>
    </div>
  );
}

import { useState, Suspense, lazy } from "react";

const WebTerminal = lazy(async () => {
  const module = await import("../components/WebTerminal");
  return { default: module.WebTerminal };
});

const terminalShells = ["bash", "zsh", "sh"];

export function OpsView({ nav, theme, t }: any) {
  const [terminalShell, setTerminalShell] = useState("bash");
  const terminalWsPath = "/api/ops/terminal/ws";

  return (
    <div className="space-y-6 fade-in">
      <div className="bg-card/50 border border-border/40 rounded-xl overflow-hidden shadow-sm backdrop-blur-sm">
        <div className="p-4 border-b border-border/40 bg-muted/20 flex items-center justify-between">
          <div>
            <h3 className="text-sm font-semibold text-foreground">交互终端 (Interactive Terminal)</h3>
            <p className="text-xs text-muted-foreground mt-1">仅保留一个交互终端。默认从仓库根目录启动。</p>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">Shell</span>
            <select
              value={terminalShell}
              onChange={(e) => setTerminalShell(e.target.value)}
              className="bg-background border border-border rounded px-2 py-1 text-xs"
            >
              {terminalShells.map(shell => (
                <option key={shell} value={shell}>{shell}</option>
              ))}
            </select>
          </div>
        </div>
        <div className="p-4 bg-black/90 min-h-[500px]">
          <Suspense fallback={<div className="text-muted-foreground text-sm flex items-center justify-center h-full">终端加载中...</div>}>
            <WebTerminal cwd="repo" shell={terminalShell} endpoint={terminalWsPath} />
          </Suspense>
        </div>
      </div>
    </div>
  );
}

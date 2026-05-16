import { useApi, postApi } from "../hooks/use-api";
import { useEffect, useState, type ReactNode } from "react";
import type { Theme } from "../hooks/use-theme";
import type { TFunction } from "../hooks/use-i18n";
import { useColors } from "../hooks/use-colors";
import type { SSEMessage } from "../hooks/use-sse";
import { shouldRefetchDaemonStatus } from "../hooks/use-book-activity";
import { Activity, AlertTriangle, CheckCircle2, Clock3, GitBranch, Loader2, PauseCircle, PlayCircle, Radio, RotateCw, XCircle } from "lucide-react";

interface Nav {
  toDashboard: () => void;
}

interface DaemonSummary {
  readonly booksTotal: number;
  readonly booksActive: number;
  readonly runsQueued: number;
  readonly runsRunning: number;
  readonly runsSucceeded: number;
  readonly runsFailed: number;
  readonly runsCancelled: number;
  readonly runsScheduler: number;
}

interface DaemonLogEntry {
  readonly time?: string;
  readonly level?: string;
  readonly message?: string;
  readonly attrs?: Record<string, unknown>;
  readonly raw?: string;
}

interface DaemonStatus {
  readonly running: boolean;
  readonly startedAt?: string;
  readonly lastPollAt?: string;
  readonly tickCount: number;
  readonly pollIntervalSec: number;
  readonly maxConcurrentBooks: number;
  readonly summary: DaemonSummary;
  readonly events: ReadonlyArray<DaemonLogEntry>;
  readonly mode: string;
}

function formatTime(value?: string): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function formatEventDetail(entry: DaemonLogEntry): string {
  const attrs = entry.attrs ?? {};
  const parts = [
    typeof attrs.bookId === "string" ? attrs.bookId : null,
    typeof attrs.chapter === "number" ? `ch ${attrs.chapter}` : null,
    typeof attrs.runId === "string" ? attrs.runId : null,
    typeof attrs.error === "string" ? attrs.error : null,
  ].filter(Boolean);
  return parts.length > 0 ? parts.join(" / ") : (entry.raw ?? "");
}

function eventTone(entry: DaemonLogEntry): string {
  const level = String(entry.level ?? "").toUpperCase();
  const message = String(entry.message ?? "").toLowerCase();
  if (level === "ERROR" || message.includes("error") || message.includes("failed")) {
    return "border-destructive/30 bg-destructive/10 text-destructive";
  }
  if (message.includes("scheduled")) {
    return "border-primary/30 bg-primary/10 text-primary";
  }
  if (message.includes("stopped")) {
    return "border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300";
  }
  return "border-border bg-secondary/40 text-muted-foreground";
}

function latestScheduledEvent(events: ReadonlyArray<DaemonLogEntry>): DaemonLogEntry | undefined {
  return events.find((entry) => String(entry.message ?? "").includes("scheduled chapter"));
}

export function DaemonControl({ nav, theme, t, sse }: { nav: Nav; theme: Theme; t: TFunction; sse: { messages: ReadonlyArray<SSEMessage> } }) {
  const c = useColors(theme);
  const { data, refetch } = useApi<DaemonStatus>("/daemon");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const recent = sse.messages.at(-1);
    if (!shouldRefetchDaemonStatus(recent)) return;
    void refetch();
  }, [refetch, sse.messages]);

  useEffect(() => {
    if (!data?.running) return;
    const id = window.setInterval(() => {
      void refetch();
    }, 5000);
    return () => window.clearInterval(id);
  }, [data?.running, refetch]);

  const daemonEvents = sse.messages
    .filter((m) => m.event.startsWith("daemon:") || m.event === "log")
    .slice(-20);

  const apiEvents = data?.events ?? [];
  const summary = data?.summary;
  const latestScheduled = latestScheduledEvent(apiEvents);

  const handleStart = async () => {
    setLoading(true);
    try {
      await postApi("/daemon/start");
      await refetch();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed");
    } finally {
      setLoading(false);
    }
  };

  const handleStop = async () => {
    setLoading(true);
    try {
      await postApi("/daemon/stop");
      await refetch();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed");
    } finally {
      setLoading(false);
    }
  };

  const handlePoll = async () => {
    setLoading(true);
    try {
      await postApi("/daemon/poll");
      await refetch();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Failed");
    } finally {
      setLoading(false);
    }
  };

  const isRunning = data?.running ?? false;

  return (
    <div className="space-y-8">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <button onClick={nav.toDashboard} className={c.link}>{t("bread.home")}</button>
        <span className="text-border">/</span>
        <span className="text-foreground">{t("nav.daemon")}</span>
      </div>

      <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div>
          <h1 className="font-serif text-3xl">{t("daemon.title")}</h1>
          <p className="mt-2 text-sm text-muted-foreground">{t("daemon.subtitle")}</p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <span className={`text-sm uppercase tracking-wide font-medium ${isRunning ? "text-emerald-500" : "text-muted-foreground"}`}>
            {isRunning ? t("daemon.running") : t("daemon.stopped")}
          </span>
          <button
            onClick={handlePoll}
            disabled={loading}
            className={`px-4 py-2.5 text-sm rounded-md ${c.btnSecondary} disabled:opacity-50 inline-flex items-center gap-2`}
          >
            <RotateCw size={15} className={loading ? "animate-spin" : ""} />
            {t("daemon.pollNow")}
          </button>
          {isRunning ? (
            <button
              onClick={handleStop}
              disabled={loading}
              className={`px-4 py-2.5 text-sm rounded-md ${c.btnDanger} disabled:opacity-50 inline-flex items-center gap-2`}
            >
              <PauseCircle size={15} />
              {loading ? t("daemon.stopping") : t("daemon.stop")}
            </button>
          ) : (
            <button
              onClick={handleStart}
              disabled={loading}
              className={`px-4 py-2.5 text-sm rounded-md ${c.btnPrimary} disabled:opacity-50 inline-flex items-center gap-2`}
            >
              <PlayCircle size={15} />
              {loading ? t("daemon.starting") : t("daemon.start")}
            </button>
          )}
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-4">
        <StatusCard
          icon={isRunning ? <Radio size={18} /> : <PauseCircle size={18} />}
          label={t("daemon.status")}
          value={isRunning ? t("daemon.running") : t("daemon.stopped")}
          hint={`${t("daemon.mode")}: ${data?.mode ?? "—"}`}
          tone={isRunning ? "text-emerald-500" : "text-muted-foreground"}
        />
        <StatusCard
          icon={<Activity size={18} />}
          label={t("daemon.activeBooks")}
          value={`${summary?.booksActive ?? 0}/${summary?.booksTotal ?? 0}`}
          hint={t("daemon.activeBooksHint")}
          tone="text-primary"
        />
        <StatusCard
          icon={<Loader2 size={18} className={summary?.runsRunning ? "animate-spin" : ""} />}
          label={t("daemon.inFlight")}
          value={`${summary?.runsRunning ?? 0} / ${summary?.runsQueued ?? 0}`}
          hint={t("daemon.runningQueued")}
          tone={summary?.runsRunning ? "text-emerald-500" : "text-muted-foreground"}
        />
        <StatusCard
          icon={<GitBranch size={18} />}
          label={t("daemon.schedulerRuns")}
          value={`${summary?.runsScheduler ?? 0}`}
          hint={`${t("daemon.capacity")}: ${data?.maxConcurrentBooks ?? "—"}`}
          tone="text-primary"
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-[1.2fr_0.8fr]">
        <div className={`border ${c.cardStatic} rounded-lg bg-card/40`}>
          <div className="px-5 py-3.5 border-b border-border flex items-center justify-between">
            <span className="text-sm uppercase tracking-wide text-muted-foreground font-medium">{t("daemon.schedulerState")}</span>
            <span className="text-xs text-muted-foreground">{t("daemon.tick")} #{data?.tickCount ?? 0}</span>
          </div>
          <div className="p-5 space-y-5">
            <div className="grid gap-3 sm:grid-cols-3">
              <Metric label={t("daemon.startedAt")} value={formatTime(data?.startedAt)} />
              <Metric label={t("daemon.lastPollAt")} value={formatTime(data?.lastPollAt)} />
              <Metric label={t("daemon.pollInterval")} value={`${data?.pollIntervalSec ?? "—"}s`} />
            </div>

            <div className="rounded-lg border border-border bg-background/50 p-4">
              <div className="flex items-center gap-3">
                <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10 text-primary">
                  <Clock3 size={18} />
                </div>
                <div className="min-w-0">
                  <p className="text-xs uppercase tracking-wide text-muted-foreground">{t("daemon.latestSchedule")}</p>
                  <p className="truncate text-sm font-medium text-foreground">
                    {latestScheduled ? formatEventDetail(latestScheduled) : t("daemon.noScheduleYet")}
                  </p>
                </div>
              </div>
            </div>

            <div className="grid gap-3 sm:grid-cols-4">
              <RunPill icon={<CheckCircle2 size={15} />} label={t("daemon.succeeded")} value={summary?.runsSucceeded ?? 0} tone="text-emerald-500" />
              <RunPill icon={<AlertTriangle size={15} />} label={t("daemon.failed")} value={summary?.runsFailed ?? 0} tone="text-destructive" />
              <RunPill icon={<XCircle size={15} />} label={t("daemon.cancelled")} value={summary?.runsCancelled ?? 0} tone="text-amber-500" />
              <RunPill icon={<Loader2 size={15} />} label={t("daemon.queued")} value={summary?.runsQueued ?? 0} tone="text-primary" />
            </div>
          </div>
        </div>

        <div className={`border ${c.cardStatic} rounded-lg bg-card/40`}>
          <div className="px-5 py-3.5 border-b border-border">
            <span className="text-sm uppercase tracking-wide text-muted-foreground font-medium">{t("daemon.recentDaemonEvents")}</span>
          </div>
          <div className="p-4 max-h-[360px] overflow-y-auto">
            {apiEvents.length > 0 ? (
              <div className="space-y-2">
                {apiEvents.slice(0, 12).map((entry, i) => (
                  <div key={`${entry.time ?? "event"}-${i}`} className={`rounded-md border px-3 py-2.5 ${eventTone(entry)}`}>
                    <div className="flex items-center justify-between gap-3">
                      <span className="truncate text-sm font-medium">{entry.message ?? t("daemon.event")}</span>
                      <span className="shrink-0 text-[11px] opacity-70">{formatTime(entry.time)}</span>
                    </div>
                    <p className="mt-1 truncate text-xs opacity-80">{formatEventDetail(entry)}</p>
                  </div>
                ))}
              </div>
            ) : (
              <EmptyState text={isRunning ? t("daemon.waitingEvents") : t("daemon.startHint")} />
            )}
          </div>
        </div>
      </div>

      <div className={`border ${c.cardStatic} rounded-lg bg-card/40`}>
        <div className="px-5 py-3.5 border-b border-border">
          <span className="text-sm uppercase tracking-wide text-muted-foreground font-medium">{t("daemon.liveEventLog")}</span>
        </div>
        <div className="p-4 max-h-[500px] overflow-y-auto">
          {daemonEvents.length > 0 ? (
            <div className="space-y-1.5 font-mono text-sm">
              {daemonEvents.map((msg, i) => {
                const d = msg.data as Record<string, unknown>;
                return (
                  <div key={i} className="leading-relaxed text-muted-foreground">
                    <span className="text-primary/50">{msg.event}</span>
                    <span className="text-border mx-1.5">›</span>
                    <span>{String(d.message ?? d.bookId ?? JSON.stringify(d))}</span>
                  </div>
                );
              })}
            </div>
          ) : (
            <EmptyState text={isRunning ? t("daemon.waitingEvents") : t("daemon.startHint")} />
          )}
        </div>
      </div>
    </div>
  );
}

function StatusCard({ icon, label, value, hint, tone }: { icon: ReactNode; label: string; value: string; hint: string; tone: string }) {
  return (
    <div className="rounded-lg border border-border bg-card/40 p-4">
      <div className={`mb-3 flex h-9 w-9 items-center justify-center rounded-full bg-secondary ${tone}`}>
        {icon}
      </div>
      <p className="text-xs uppercase tracking-wide text-muted-foreground">{label}</p>
      <p className="mt-1 text-2xl font-semibold text-foreground">{value}</p>
      <p className="mt-1 text-xs text-muted-foreground">{hint}</p>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-border bg-secondary/30 px-3 py-2.5">
      <p className="text-[11px] uppercase tracking-wide text-muted-foreground">{label}</p>
      <p className="mt-1 truncate text-sm font-medium text-foreground">{value}</p>
    </div>
  );
}

function RunPill({ icon, label, value, tone }: { icon: ReactNode; label: string; value: number; tone: string }) {
  return (
    <div className="rounded-md border border-border bg-secondary/30 px-3 py-2.5">
      <div className={`flex items-center gap-2 ${tone}`}>
        {icon}
        <span className="text-lg font-semibold">{value}</span>
      </div>
      <p className="mt-1 text-xs text-muted-foreground">{label}</p>
    </div>
  );
}

function EmptyState({ text }: { text: string }) {
  return (
    <div className="text-muted-foreground text-sm italic py-8 text-center">
      {text}
    </div>
  );
}

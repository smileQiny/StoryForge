import { useApi } from "../hooks/use-api";
import type { Theme } from "../hooks/use-theme";
import type { TFunction } from "../hooks/use-i18n";
import { useColors } from "../hooks/use-colors";
import { Stethoscope, CheckCircle2, XCircle, Loader2 } from "lucide-react";

interface DoctorChecks {
  readonly configJson?: boolean;
  readonly projectEnv?: boolean;
  readonly globalEnv?: boolean;
  readonly booksDir?: boolean;
  readonly llmConnected?: boolean;
  readonly bookCount?: number;
}

interface Nav { toDashboard: () => void }

function CheckRow({ label, ok, detail }: { label: string; ok: boolean; detail?: string }) {
  return (
    <div className="flex items-center gap-3 py-3 border-b border-border/30 last:border-0">
      {ok ? (
        <CheckCircle2 size={18} className="text-emerald-500 shrink-0" />
      ) : (
        <XCircle size={18} className="text-destructive shrink-0" />
      )}
      <span className="text-sm font-medium flex-1">{label}</span>
      {detail && <span className="text-xs text-muted-foreground">{detail}</span>}
    </div>
  );
}

export function DoctorView({ nav, theme, t }: { nav: Nav; theme: Theme; t: TFunction }) {
  const c = useColors(theme);
  const { data, loading, error, refetch } = useApi<DoctorChecks>("/doctor");
  const configReady = data?.configJson ?? false;
  const envReady = Boolean(data?.projectEnv || data?.globalEnv);
  const llmReady = Boolean(data?.llmConnected);

  return (
    <div className="space-y-8">
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <button onClick={nav.toDashboard} className={c.link}>{t("bread.home")}</button>
        <span className="text-border">/</span>
        <span>{t("nav.doctor")}</span>
      </div>

      <div className="flex items-center justify-between">
        <h1 className="font-serif text-3xl flex items-center gap-3">
          <Stethoscope size={28} className="text-primary" />
          {t("doctor.title")}
        </h1>
        <button onClick={() => refetch()} className={`px-4 py-2 text-sm rounded-lg ${c.btnSecondary}`}>
          {t("doctor.recheck")}
        </button>
      </div>

      {loading && !data ? (
        <div className="flex items-center justify-center py-12">
          <Loader2 size={24} className="animate-spin text-primary" />
        </div>
      ) : error ? (
        <div className="rounded-lg border border-destructive/20 bg-destructive/5 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      ) : (
        <div className={`border ${c.cardStatic} rounded-lg p-5`}>
          <CheckRow label={t("doctor.configJson")} ok={configReady} />
          <CheckRow label={t("doctor.projectEnv")} ok={Boolean(data?.projectEnv)} />
          <CheckRow label={t("doctor.globalEnv")} ok={Boolean(data?.globalEnv)} />
          <CheckRow label={t("doctor.booksDir")} ok={Boolean(data?.booksDir)} detail={`${data?.bookCount ?? 0} book(s)`} />
          <CheckRow label={t("doctor.llmApi")} ok={llmReady} detail={llmReady ? t("doctor.connected") : t("doctor.failed")} />
        </div>
      )}

      {data && (
        <div className={`px-4 py-3 rounded-lg text-sm font-medium ${
          configReady && envReady && llmReady
            ? "bg-emerald-500/10 text-emerald-600"
            : "bg-amber-500/10 text-amber-600"
        }`}>
          {configReady && envReady && llmReady
            ? t("doctor.allPassed")
            : t("doctor.someFailed")
          }
        </div>
      )}
    </div>
  );
}

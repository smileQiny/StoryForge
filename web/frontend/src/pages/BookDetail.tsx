import { fetchJson, useApi, postApi } from "../hooks/use-api";
import { useEffect, useMemo, useState } from "react";
import type { Theme } from "../hooks/use-theme";
import type { TFunction } from "../hooks/use-i18n";
import type { SSEMessage } from "../hooks/use-sse";
import { deriveBookActivity, shouldRefetchBookView } from "../hooks/use-book-activity";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { summarizeBootstrapSnapshot, type BootstrapArtifactSnapshot } from "../lib/book-bootstrap";
import { buildBookStatusOptions, getBookStatusLabel, getBookStatusTone, type BookStatus } from "../lib/book-status";
import type { ChapterAnalyzeResult } from "../lib/chapter-analysis";
import { summarizeChapterAnalysis } from "../lib/chapter-analysis";
import {
  BarChart2,
  BrainCircuit,
  Check,
  CheckCheck,
  ChevronLeft,
  Database,
  Download,
  Eye,
  FileText,
  Pencil,
  RotateCcw,
  Save,
  ShieldCheck,
  Sparkles,
  Trash2,
  Wand2,
  X,
  Zap,
} from "lucide-react";

interface ChapterMeta {
  readonly number: number;
  readonly title: string;
  readonly status: string;
  readonly wordCount: number;
}

interface BookData {
  readonly id: string;
  readonly title: string;
  readonly genre: string;
  readonly status: string;
  readonly chapterWordCount: number;
  readonly targetChapters?: number;
  readonly language?: string;
  readonly fanficMode?: string;
}

type ReviseMode = "spot-fix" | "polish" | "rewrite" | "rework" | "anti-detect";
type ExportFormat = "txt" | "md" | "epub";
type FoundationFileName = "current_state.json" | "pending_hooks.json";

interface Nav {
  toDashboard: () => void;
  toChapter: (bookId: string, num: number) => void;
  toAnalytics: (bookId: string) => void;
  toTruth?: (bookId: string) => void;
}

function translateChapterStatus(status: string, t: TFunction): string {
  const map: Record<string, () => string> = {
    "ready-for-review": () => t("chapter.readyForReview"),
    "pending-review": () => t("chapter.readyForReview"),
    approved: () => t("chapter.approved"),
    revised: () => t("chapter.revised"),
    drafted: () => t("chapter.drafted"),
    draft: () => t("chapter.drafted"),
    "needs-revision": () => t("chapter.needsRevision"),
    imported: () => t("chapter.imported"),
    "audit-failed": () => t("chapter.auditFailed"),
    rejected: () => t("chapter.rejected"),
    audited: () => t("chapter.audited"),
  };
  return map[status]?.() ?? status;
}

const CHAPTER_STATUS_TONE: Record<string, string> = {
  "ready-for-review": "bg-amber-500/10 text-amber-700 dark:text-amber-300",
  "pending-review": "bg-amber-500/10 text-amber-700 dark:text-amber-300",
  approved: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
  revised: "bg-sky-500/10 text-sky-700 dark:text-sky-300",
  draft: "bg-secondary text-muted-foreground",
  drafted: "bg-secondary text-muted-foreground",
  "needs-revision": "bg-destructive/10 text-destructive",
  imported: "bg-sky-500/10 text-sky-700 dark:text-sky-300",
  "audit-failed": "bg-destructive/10 text-destructive",
  rejected: "bg-destructive/10 text-destructive",
  audited: "bg-primary/10 text-primary",
};

function isReviewableChapterStatus(status: string): boolean {
  return status === "pending-review" || status === "ready-for-review";
}

function StatBlock({ label, value }: { readonly label: string; readonly value: string }) {
  return (
    <div>
      <div className="text-[11px] font-semibold uppercase text-muted-foreground">{label}</div>
      <div className="mt-1 text-lg font-semibold text-foreground">{value}</div>
    </div>
  );
}

function SectionHeading({ title, description }: { readonly title: string; readonly description?: string }) {
  return (
    <div className="mb-4">
      <h2 className="text-sm font-semibold text-foreground">{title}</h2>
      {description ? <p className="mt-1 text-xs text-muted-foreground">{description}</p> : null}
    </div>
  );
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null;
}

function formatJson(value: unknown): string {
  return JSON.stringify(value ?? {}, null, 2);
}

function getFoundationArtifactPayload(truthData: Record<string, unknown> | undefined, artifact: BootstrapArtifactSnapshot): unknown {
  if (artifact.key === "initialHooks") {
    return truthData?.pendingHooks ?? [];
  }
  if (artifact.key === "initialState") {
    return truthData?.currentState ?? {};
  }
  const currentState = asRecord(truthData?.currentState);
  const foundation = asRecord(currentState?.foundation);
  return foundation?.[artifact.key] ?? {};
}

export function BookDetail({
  bookId,
  nav,
  theme: _theme,
  t,
  sse,
}: {
  bookId: string;
  nav: Nav;
  theme: Theme;
  t: TFunction;
  sse: { messages: ReadonlyArray<SSEMessage> };
}) {
  const { data: book, loading: loadingBook, error: bookError, refetch: refetchBook } = useApi<BookData>(`/books/${bookId}`);
  const { data: chaptersData, loading: loadingChapters, error: chaptersError, refetch: refetchChapters } = useApi<ReadonlyArray<ChapterMeta>>(`/books/${bookId}/chapters`);
  const { data: truthData, refetch: refetchTruth } = useApi<Record<string, unknown>>(`/books/${bookId}/truth`);
  const [writeRequestPending, setWriteRequestPending] = useState(false);
  const [draftRequestPending, setDraftRequestPending] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false);
  const [rewritingChapters, setRewritingChapters] = useState<ReadonlyArray<number>>([]);
  const [revisingChapters, setRevisingChapters] = useState<ReadonlyArray<number>>([]);
  const [analyzingChapters, setAnalyzingChapters] = useState<ReadonlyArray<number>>([]);
  const [chapterAnalyses, setChapterAnalyses] = useState<Record<number, ChapterAnalyzeResult | undefined>>({});
  const [chapterAnalysisErrors, setChapterAnalysisErrors] = useState<Record<number, string | undefined>>({});
  const [savingSettings, setSavingSettings] = useState(false);
  const [settingsWordCount, setSettingsWordCount] = useState<number | null>(null);
  const [settingsTargetChapters, setSettingsTargetChapters] = useState<number | null>(null);
  const [settingsStatus, setSettingsStatus] = useState<BookStatus | null>(null);
  const [exportFormat, setExportFormat] = useState<ExportFormat>("txt");
  const [exportApprovedOnly, setExportApprovedOnly] = useState(false);
  const [selectedArtifact, setSelectedArtifact] = useState<BootstrapArtifactSnapshot | null>(null);
  const [artifactEditText, setArtifactEditText] = useState("");
  const [savingArtifact, setSavingArtifact] = useState(false);
  const activity = useMemo(() => deriveBookActivity(sse.messages, bookId), [bookId, sse.messages]);
  const writing = writeRequestPending || activity.writing;
  const drafting = draftRequestPending || activity.drafting;

  useEffect(() => {
    const recent = sse.messages.at(-1);
    if (!recent) return;

    const eventData = recent.data as { bookId?: string } | null;
    if (eventData?.bookId !== bookId) return;

    if (recent.event === "write:start") {
      setWriteRequestPending(false);
      return;
    }

    if (recent.event === "draft:start") {
      setDraftRequestPending(false);
      return;
    }

    if (shouldRefetchBookView(recent, bookId)) {
      setWriteRequestPending(false);
      setDraftRequestPending(false);
      void Promise.all([refetchBook(), refetchChapters(), refetchTruth()]);
    }
  }, [bookId, refetchBook, refetchChapters, refetchTruth, sse.messages]);

  const handleWriteNext = async () => {
    setWriteRequestPending(true);
    try {
      await postApi(`/books/${bookId}/write-next`);
    } catch (e) {
      setWriteRequestPending(false);
      alert(e instanceof Error ? e.message : "Failed");
    }
  };

  const handleDraft = async () => {
    setDraftRequestPending(true);
    try {
      await postApi(`/books/${bookId}/draft`);
    } catch (e) {
      setDraftRequestPending(false);
      alert(e instanceof Error ? e.message : "Failed");
    }
  };

  const handleDeleteBook = async () => {
    setConfirmDeleteOpen(false);
    setDeleting(true);
    try {
      const res = await fetch(`/api/books/${bookId}`, { method: "DELETE" });
      if (!res.ok) {
        const json = await res.json().catch(() => ({}));
        throw new Error((json as { error?: string }).error ?? `${res.status}`);
      }
      nav.toDashboard();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Delete failed");
    } finally {
      setDeleting(false);
    }
  };

  const handleRewrite = async (chapterNum: number) => {
    setRewritingChapters((prev) => [...prev, chapterNum]);
    try {
      await postApi(`/books/${bookId}/rewrite/${chapterNum}`);
      await refetchChapters();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Rewrite failed");
    } finally {
      setRewritingChapters((prev) => prev.filter((n) => n !== chapterNum));
    }
  };

  const handleRevise = async (chapterNum: number, mode: ReviseMode) => {
    setRevisingChapters((prev) => [...prev, chapterNum]);
    try {
      await fetchJson(`/books/${bookId}/revise/${chapterNum}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ mode }),
      });
      await refetchChapters();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Revision failed");
    } finally {
      setRevisingChapters((prev) => prev.filter((n) => n !== chapterNum));
    }
  };

  const handleAnalyzeChapter = async (chapterNum: number) => {
    setAnalyzingChapters((prev) => [...prev, chapterNum]);
    setChapterAnalysisErrors((prev) => ({ ...prev, [chapterNum]: undefined }));
    try {
      const result = await postApi<ChapterAnalyzeResult>(`/books/${bookId}/chapters/${chapterNum}/analyze`);
      setChapterAnalyses((prev) => ({ ...prev, [chapterNum]: result }));
    } catch (e) {
      setChapterAnalysisErrors((prev) => ({
        ...prev,
        [chapterNum]: e instanceof Error ? e.message : "Analyze failed",
      }));
    } finally {
      setAnalyzingChapters((prev) => prev.filter((n) => n !== chapterNum));
    }
  };

  const openChapterEditor = (chapterNum: number) => {
    window.sessionStorage.setItem(`storyforge:edit-chapter:${bookId}:${chapterNum}`, "1");
    nav.toChapter(bookId, chapterNum);
  };

  const handleSaveSettings = async () => {
    if (!book) return;
    setSavingSettings(true);
    try {
      const body: Record<string, unknown> = {};
      if (settingsWordCount !== null) body.chapterWordCount = settingsWordCount;
      if (settingsTargetChapters !== null) body.targetChapters = settingsTargetChapters;
      if (settingsStatus !== null) body.status = settingsStatus;
      await fetchJson(`/books/${bookId}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      await refetchBook();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSavingSettings(false);
    }
  };

  const openFoundationArtifact = (artifact: BootstrapArtifactSnapshot) => {
    setSelectedArtifact(artifact);
    setArtifactEditText(formatJson(getFoundationArtifactPayload(truthData, artifact)));
  };

  const saveFoundationArtifact = async () => {
    if (!selectedArtifact) return;
    setSavingArtifact(true);
    try {
      const parsed = JSON.parse(artifactEditText);
      let file: FoundationFileName = "current_state.json";
      let payload: unknown = parsed;

      if (selectedArtifact.key === "initialHooks") {
        file = "pending_hooks.json";
      } else if (selectedArtifact.key !== "initialState") {
        const currentState = {
          ...(asRecord(truthData?.currentState) ?? {}),
        };
        const foundation = {
          ...(asRecord(currentState.foundation) ?? {}),
          [selectedArtifact.key]: parsed,
        };
        currentState.foundation = foundation;
        payload = currentState;
      }

      await fetchJson(`/books/${bookId}/truth/${file}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      await refetchTruth();
      setSelectedArtifact(null);
    } catch (e) {
      alert(e instanceof Error ? e.message : "Save foundation artifact failed");
    } finally {
      setSavingArtifact(false);
    }
  };

  const handleApproveAll = async () => {
    const reviewable = chapters.filter((chapter) => isReviewableChapterStatus(chapter.status));
    for (const chapter of reviewable) {
      await postApi(`/books/${bookId}/chapters/${chapter.number}/approve`);
    }
    await refetchChapters();
  };

  if (loadingBook || loadingChapters) {
    return (
      <div className="flex min-h-[50vh] flex-col items-center justify-center gap-3">
        <div className="h-8 w-8 rounded-full border-2 border-primary/20 border-t-primary animate-spin" />
        <span className="text-sm text-muted-foreground">{t("common.loading")}</span>
      </div>
    );
  }

  if (bookError || chaptersError) {
    return <div className="rounded-md border border-destructive/20 bg-destructive/5 p-6 text-sm text-destructive">Error: {bookError ?? chaptersError}</div>;
  }

  if (!book) return null;

  const chapters = chaptersData ?? [];
  const nextChapter = chapters.reduce((maxNumber, chapter) => Math.max(maxNumber, chapter.number), 0) + 1;
  const totalWords = chapters.reduce((sum, chapter) => sum + (chapter.wordCount ?? 0), 0);
  const reviewCount = chapters.filter((chapter) => isReviewableChapterStatus(chapter.status)).length;
  const approvedCount = chapters.filter((chapter) => chapter.status === "approved").length;
  const currentWordCount = settingsWordCount ?? book.chapterWordCount;
  const currentTargetChapters = settingsTargetChapters ?? book.targetChapters ?? 0;
  const currentStatus = settingsStatus ?? (book.status as BookStatus);
  const bootstrap = summarizeBootstrapSnapshot(truthData);
  const bookStatusLabel = getBookStatusLabel(currentStatus, t);
  const bookStatusOptions = buildBookStatusOptions(t);

  return (
    <div className="space-y-8">
      <nav className="flex items-center gap-2 text-[13px] text-muted-foreground">
        <button
          onClick={nav.toDashboard}
          className="inline-flex items-center gap-1 hover:text-foreground transition-colors"
        >
          <ChevronLeft size={14} />
          {t("bread.books")}
        </button>
        <span>/</span>
        <span className="truncate text-foreground">{book.title}</span>
      </nav>

      <section className="border-b border-border/70 pb-6">
        <div className="flex flex-col gap-5 xl:flex-row xl:items-start xl:justify-between">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="truncate text-3xl font-semibold text-foreground">{book.title}</h1>
              {book.language === "en" ? (
                <span className="rounded-sm border border-primary/20 px-2 py-1 text-xs font-semibold text-primary">EN</span>
              ) : null}
              <span className={`rounded-sm px-2 py-1 text-xs font-semibold ${getBookStatusTone(currentStatus)}`}>
                {bookStatusLabel}
              </span>
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span className="rounded-sm bg-secondary px-2 py-1">{book.genre}</span>
              <span className="rounded-sm bg-secondary px-2 py-1">{chapters.length} {t("dash.chapters")}</span>
              <span className="rounded-sm bg-secondary px-2 py-1">{totalWords.toLocaleString()} {t("book.words")}</span>
              <span className="rounded-sm bg-secondary px-2 py-1">Next {nextChapter}</span>
              {book.fanficMode ? (
                <span className="inline-flex items-center gap-1 rounded-sm bg-secondary px-2 py-1">
                  <Sparkles size={12} />
                  {book.fanficMode}
                </span>
              ) : null}
            </div>
          </div>

          <div className="flex flex-wrap gap-2">
            <button
              onClick={handleWriteNext}
              disabled={writing || drafting}
              className="inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2.5 text-sm font-semibold text-primary-foreground transition-colors hover:opacity-90 disabled:opacity-60"
            >
              {writing ? <div className="h-4 w-4 rounded-full border-2 border-primary-foreground/25 border-t-primary-foreground animate-spin" /> : <Zap size={16} />}
              {writing ? t("dash.writing") : t("book.writeNext")}
            </button>
            <button
              onClick={handleDraft}
              disabled={writing || drafting}
              className="inline-flex items-center gap-2 rounded-md border border-border bg-secondary px-4 py-2.5 text-sm font-semibold text-foreground transition-colors hover:bg-background disabled:opacity-60"
            >
              {drafting ? <div className="h-4 w-4 rounded-full border-2 border-muted-foreground/25 border-t-muted-foreground animate-spin" /> : <Wand2 size={16} />}
              {drafting ? t("book.drafting") : t("book.draftOnly")}
            </button>
            <button
              onClick={() => setConfirmDeleteOpen(true)}
              disabled={deleting}
              className="inline-flex items-center gap-2 rounded-md border border-destructive/20 bg-destructive/10 px-4 py-2.5 text-sm font-semibold text-destructive transition-colors hover:bg-destructive/15 disabled:opacity-60"
            >
              {deleting ? <div className="h-4 w-4 rounded-full border-2 border-destructive/25 border-t-destructive animate-spin" /> : <Trash2 size={16} />}
              {t("book.deleteBook")}
            </button>
          </div>
        </div>
      </section>

      {(writing || drafting || activity.lastError) ? (
        <section className={`rounded-md border px-4 py-3 text-sm ${
          activity.lastError
            ? "border-destructive/20 bg-destructive/5 text-destructive"
            : "border-primary/20 bg-primary/[0.04] text-foreground"
        }`}>
          {activity.lastError ? `${t("book.pipelineFailed")}: ${activity.lastError}` : writing ? t("book.pipelineWriting") : t("book.pipelineDrafting")}
        </section>
      ) : null}

      <section className="grid gap-4 border-b border-border/70 pb-6 sm:grid-cols-2 xl:grid-cols-4">
        <StatBlock label={t("dash.chapters")} value={String(chapters.length)} />
        <StatBlock label={t("book.words")} value={totalWords.toLocaleString()} />
        <StatBlock label={t("chapter.readyForReview")} value={String(reviewCount)} />
        <StatBlock label={t("chapter.approved")} value={String(approvedCount)} />
      </section>

      <section className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="space-y-6">
          <div className="rounded-md border border-border/70 bg-card p-4 md:p-5">
            <SectionHeading title={t("book.curate")} description="Library actions, exports, and related views." />
            <div className="flex flex-wrap items-center gap-2">
              {reviewCount > 0 ? (
                <button
                  onClick={handleApproveAll}
                  className="inline-flex items-center gap-2 rounded-md bg-emerald-500/10 px-3 py-2 text-sm font-semibold text-emerald-700 transition-colors hover:bg-emerald-500/15 dark:text-emerald-300"
                >
                  <CheckCheck size={15} />
                  {t("book.approveAll")} ({reviewCount})
                </button>
              ) : null}
              <button
                onClick={() => nav.toTruth?.(bookId)}
                className="inline-flex items-center gap-2 rounded-md border border-border bg-secondary px-3 py-2 text-sm font-medium text-foreground transition-colors hover:bg-background"
              >
                <Database size={15} />
                {bootstrap?.truthFileCount ? `${t("book.truthFiles")} (${bootstrap.truthFileCount})` : t("book.truthFiles")}
              </button>
              <button
                onClick={() => nav.toAnalytics(bookId)}
                className="inline-flex items-center gap-2 rounded-md border border-border bg-secondary px-3 py-2 text-sm font-medium text-foreground transition-colors hover:bg-background"
              >
                <BarChart2 size={15} />
                {t("book.analytics")}
              </button>
              <div className="flex flex-wrap items-center gap-2">
                <select
                  value={exportFormat}
                  onChange={(e) => setExportFormat(e.target.value as ExportFormat)}
                  className="rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-primary"
                >
                  <option value="txt">TXT</option>
                  <option value="md">MD</option>
                  <option value="epub">EPUB</option>
                </select>
                <label className="inline-flex items-center gap-2 text-sm text-muted-foreground">
                  <input
                    type="checkbox"
                    checked={exportApprovedOnly}
                    onChange={(e) => setExportApprovedOnly(e.target.checked)}
                    className="rounded border-border"
                  />
                  {t("book.approvedOnly")}
                </label>
                <button
                  onClick={async () => {
                    try {
                      const exportData = await fetchJson<{ path?: string; chapters?: number }>(`/books/${bookId}/export-save`, {
                        method: "POST",
                        headers: { "Content-Type": "application/json" },
                        body: JSON.stringify({ format: exportFormat, approvedOnly: exportApprovedOnly }),
                      });
                      alert(`${t("common.exportSuccess")}\n${exportData.path}\n(${exportData.chapters} ${t("dash.chapters")})`);
                    } catch (e) {
                      alert(e instanceof Error ? e.message : "Export failed");
                    }
                  }}
                  className="inline-flex items-center gap-2 rounded-md border border-border bg-secondary px-3 py-2 text-sm font-medium text-foreground transition-colors hover:bg-background"
                >
                  <Download size={15} />
                  {t("book.export")}
                </button>
              </div>
            </div>
          </div>

          {bootstrap ? (
            <div className="rounded-md border border-border/70 bg-card p-4 md:p-5">
              <SectionHeading title={t("book.foundationReady")} description={t("book.foundationSummary")} />
              <div className="grid gap-4 border-b border-border/70 pb-4 sm:grid-cols-3">
                <StatBlock label={t("book.bootstrapSource")} value={bootstrap.source === "llm" ? "LLM" : "Fallback"} />
                <StatBlock label={t("book.openHooks")} value={String(bootstrap.openHookCount)} />
                <StatBlock label={t("book.truthFiles")} value={String(bootstrap.truthFileCount)} />
              </div>
              <div className="mt-4 space-y-4 text-sm">
                {bootstrap.brief ? (
                  <div>
                    <div className="text-[11px] font-semibold uppercase text-muted-foreground">{t("book.foundationBrief")}</div>
                    <p className="mt-1 text-foreground">{bootstrap.brief}</p>
                  </div>
                ) : null}
                {bootstrap.currentFocus ? (
                  <div>
                    <div className="text-[11px] font-semibold uppercase text-muted-foreground">{t("book.currentFocus")}</div>
                    <p className="mt-1 text-foreground">{bootstrap.currentFocus}</p>
                  </div>
                ) : null}
                {bootstrap.coreConflict ? (
                  <div>
                    <div className="text-[11px] font-semibold uppercase text-muted-foreground">{t("book.foundationConflict")}</div>
                    <p className="mt-1 text-foreground">{bootstrap.coreConflict}</p>
                  </div>
                ) : null}
                {bootstrap.worldAnchor ? (
                  <div>
                    <div className="text-[11px] font-semibold uppercase text-muted-foreground">{t("book.foundationWorld")}</div>
                    <p className="mt-1 text-foreground">{bootstrap.worldAnchor}</p>
                  </div>
                ) : null}
                {bootstrap.artifacts.length ? (
                  <div>
                    <div className="text-[11px] font-semibold uppercase text-muted-foreground">{t("book.foundationArtifacts")}</div>
                    <div className="mt-2 grid gap-2 sm:grid-cols-2">
                      {bootstrap.artifacts.map((artifact) => (
                        <button
                          key={artifact.key}
                          onClick={() => openFoundationArtifact(artifact)}
                          className="rounded-md border border-border/70 bg-background/60 px-3 py-3 text-left transition-colors hover:border-primary/40 hover:bg-primary/[0.03]"
                        >
                          <div className="flex items-start justify-between gap-3">
                            <div className="text-sm font-semibold text-foreground">{artifact.title}</div>
                            <span className="inline-flex items-center gap-1 rounded-sm bg-secondary px-2 py-1 text-[11px] text-muted-foreground">
                              <Pencil size={12} />
                              {t("book.viewEdit")}
                            </span>
                          </div>
                          {artifact.jobTitle ? (
                            <div className="mt-1 text-xs text-primary">{artifact.jobTitle}</div>
                          ) : null}
                          {artifact.responsibility ? (
                            <p className="mt-2 text-xs leading-5 text-muted-foreground">{artifact.responsibility}</p>
                          ) : null}
                          {artifact.backingFiles.length ? (
                            <div className="mt-2 truncate font-mono text-[11px] text-muted-foreground/70">
                              {artifact.backingFiles.join(" / ")}
                            </div>
                          ) : null}
                        </button>
                      ))}
                    </div>
                  </div>
                ) : null}
              </div>
            </div>
          ) : null}

          <div className="rounded-md border border-border/70 bg-card">
            <div className="border-b border-border/70 px-4 py-3 md:px-5">
              <SectionHeading title={t("reader.chapterList")} description="Read, review, revise, or regenerate each chapter." />
            </div>
            {chapters.length ? (
              <div className="divide-y divide-border/70">
                {chapters.map((chapter) => {
                  const isRewriting = rewritingChapters.includes(chapter.number);
                  const isRevising = revisingChapters.includes(chapter.number);
                  const isAnalyzing = analyzingChapters.includes(chapter.number);
                  const analysis = chapterAnalyses[chapter.number];
                  const analysisError = chapterAnalysisErrors[chapter.number];
                  const analysisSummary = summarizeChapterAnalysis(analysis);
                  return (
                    <div key={chapter.number} className="grid gap-4 px-4 py-4 lg:grid-cols-[minmax(0,1fr)_minmax(360px,0.95fr)] md:px-5">
                      <div className="min-w-0">
                        <button
                          onClick={() => nav.toChapter(bookId, chapter.number)}
                          className="truncate text-left text-base font-semibold text-foreground transition-colors hover:text-primary"
                        >
                          {chapter.title || t("chapter.label").replace("{n}", String(chapter.number))}
                        </button>
                        <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                          <span className="rounded-sm bg-secondary px-2 py-1">#{chapter.number.toString().padStart(2, "0")}</span>
                          <span className="rounded-sm bg-secondary px-2 py-1">{(chapter.wordCount ?? 0).toLocaleString()} {t("book.words")}</span>
                          <span className={`rounded-sm px-2 py-1 ${CHAPTER_STATUS_TONE[chapter.status] ?? "bg-secondary text-muted-foreground"}`}>
                            {translateChapterStatus(chapter.status, t)}
                          </span>
                        </div>
                      </div>

                      <div className="rounded-lg border border-border/60 bg-background/50 p-3">
                        <div className="mb-2 flex flex-wrap justify-start gap-2 lg:justify-end">
                          <button
                            onClick={() => nav.toChapter(bookId, chapter.number)}
                            className="inline-flex items-center gap-2 rounded-md border border-border bg-background px-3 py-2 text-sm font-medium text-foreground transition-colors hover:bg-secondary"
                          >
                            <Eye size={14} />
                            {t("reader.preview")}
                          </button>
                          <button
                            onClick={() => openChapterEditor(chapter.number)}
                            className="inline-flex items-center gap-2 rounded-md border border-border bg-background px-3 py-2 text-sm font-medium text-foreground transition-colors hover:bg-secondary"
                          >
                            <Pencil size={14} />
                            {t("reader.edit")}
                          </button>
                        </div>

                        <div className="flex flex-wrap items-start justify-start gap-2 lg:justify-end">
                        {isReviewableChapterStatus(chapter.status) ? (
                          <>
                            <button
                              onClick={async () => {
                                await postApi(`/books/${bookId}/chapters/${chapter.number}/approve`);
                                await refetchChapters();
                              }}
                              className="inline-flex items-center gap-2 rounded-md bg-emerald-500/10 px-3 py-2 text-sm font-medium text-emerald-700 transition-colors hover:bg-emerald-500/15 dark:text-emerald-300"
                            >
                              <Check size={14} />
                              {t("book.approve")}
                            </button>
                            <button
                              onClick={async () => {
                                await postApi(`/books/${bookId}/chapters/${chapter.number}/reject`);
                                await refetchChapters();
                              }}
                              className="inline-flex items-center gap-2 rounded-md bg-destructive/10 px-3 py-2 text-sm font-medium text-destructive transition-colors hover:bg-destructive/15"
                            >
                              <X size={14} />
                              {t("book.reject")}
                            </button>
                          </>
                        ) : null}
                        <button
                          onClick={() => void handleAnalyzeChapter(chapter.number)}
                          disabled={isAnalyzing}
                          className="inline-flex items-center gap-2 rounded-md border border-border bg-secondary px-3 py-2 text-sm font-medium text-foreground transition-colors hover:bg-background disabled:opacity-60"
                        >
                          {isAnalyzing ? <div className="h-4 w-4 rounded-full border-2 border-muted-foreground/25 border-t-muted-foreground animate-spin" /> : <BrainCircuit size={14} />}
                          {t("reader.analyze")}
                        </button>
                        <button
                          onClick={async () => {
                            const auditResult = await fetchJson<{ passed?: boolean; issues?: unknown[] }>(`/books/${bookId}/audit/${chapter.number}`, { method: "POST" });
                            alert(auditResult.passed ? "Audit passed" : `Audit failed: ${auditResult.issues?.length ?? 0} issues`);
                            await refetchChapters();
                          }}
                          className="inline-flex items-center gap-2 rounded-md border border-border bg-secondary px-3 py-2 text-sm font-medium text-foreground transition-colors hover:bg-background"
                        >
                          <ShieldCheck size={14} />
                          {t("book.audit")}
                        </button>
                        <button
                          onClick={() => handleRewrite(chapter.number)}
                          disabled={isRewriting}
                          className="inline-flex items-center gap-2 rounded-md border border-border bg-secondary px-3 py-2 text-sm font-medium text-foreground transition-colors hover:bg-background disabled:opacity-60"
                        >
                          {isRewriting ? <div className="h-4 w-4 rounded-full border-2 border-muted-foreground/25 border-t-muted-foreground animate-spin" /> : <RotateCcw size={14} />}
                          {t("book.rewrite")}
                        </button>
                        <select
                          disabled={isRevising}
                          value=""
                          onChange={(e) => {
                            const mode = e.target.value as ReviseMode;
                            if (mode) {
                              void handleRevise(chapter.number, mode);
                              e.currentTarget.value = "";
                            }
                          }}
                          className="rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-primary disabled:opacity-60"
                        >
                          <option value="" disabled>{isRevising ? t("common.loading") : t("book.curate")}</option>
                          <option value="spot-fix">{t("book.spotFix")}</option>
                          <option value="polish">{t("book.polish")}</option>
                          <option value="rewrite">{t("book.rewrite")}</option>
                          <option value="rework">{t("book.rework")}</option>
                          <option value="anti-detect">{t("book.antiDetect")}</option>
                        </select>
                        </div>
                      </div>

                      {(analysis || analysisError) ? (
                        <div className="md:col-span-2 rounded-md border border-border/70 bg-background px-3 py-3">
                          {analysisError ? (
                            <div className="text-xs text-destructive">{analysisError}</div>
                          ) : analysis ? (
                            <div className="space-y-3">
                              <div className="grid grid-cols-2 gap-3 md:grid-cols-6">
                                <StatBlock label={t("reader.facts")} value={analysisSummary.factCount.toLocaleString()} />
                                <StatBlock label={t("reader.newHooks")} value={analysisSummary.hookCount.toLocaleString()} />
                                <StatBlock label={t("reader.subplots")} value={analysisSummary.subplotCount.toLocaleString()} />
                                <StatBlock label={t("reader.emotionalArcs")} value={analysisSummary.emotionalArcCount.toLocaleString()} />
                                <StatBlock label={t("reader.characterMatrix")} value={analysisSummary.matrixCount.toLocaleString()} />
                                <StatBlock label={t("reader.stateTransition")} value={analysis.nextState ? t("reader.ready") : t("reader.pending")} />
                              </div>
                              {analysis.previousSummary ? (
                                <p className="text-xs leading-6 text-muted-foreground">{analysis.previousSummary}</p>
                              ) : null}
                            </div>
                          ) : null}
                        </div>
                      ) : null}
                    </div>
                  );
                })}
              </div>
            ) : (
              <div className="flex flex-col items-center justify-center px-6 py-16 text-center">
                <div className="flex h-12 w-12 items-center justify-center rounded-md border border-border bg-secondary text-muted-foreground">
                  <FileText size={18} />
                </div>
                <p className="mt-4 text-sm text-muted-foreground">{t("book.noChapters")}</p>
              </div>
            )}
          </div>
        </div>

        <aside className="space-y-6">
          <div className="rounded-md border border-border/70 bg-card p-4 md:p-5">
            <SectionHeading title={t("book.settings")} description="Update generation defaults and publication status." />
            <div className="space-y-4">
              <div className="space-y-1.5">
                <label className="text-[11px] font-semibold uppercase text-muted-foreground">{t("create.wordsPerChapter")}</label>
                <input
                  type="number"
                  value={currentWordCount}
                  onChange={(e) => setSettingsWordCount(Number(e.target.value))}
                  className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-primary"
                />
              </div>
              <div className="space-y-1.5">
                <label className="text-[11px] font-semibold uppercase text-muted-foreground">{t("create.targetChapters")}</label>
                <input
                  type="number"
                  value={currentTargetChapters}
                  onChange={(e) => setSettingsTargetChapters(Number(e.target.value))}
                  className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-primary"
                />
              </div>
              <div className="space-y-1.5">
                <label className="text-[11px] font-semibold uppercase text-muted-foreground">{t("book.status")}</label>
                <select
                  value={currentStatus}
                  onChange={(e) => setSettingsStatus(e.target.value as BookStatus)}
                  className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-primary"
                >
                  {bookStatusOptions.map((option) => (
                    <option key={option.value} value={option.value}>{option.label}</option>
                  ))}
                </select>
              </div>
              <button
                onClick={handleSaveSettings}
                disabled={savingSettings}
                className="inline-flex w-full items-center justify-center gap-2 rounded-md bg-primary px-4 py-2.5 text-sm font-semibold text-primary-foreground transition-colors hover:opacity-90 disabled:opacity-60"
              >
                {savingSettings ? <div className="h-4 w-4 rounded-full border-2 border-primary-foreground/25 border-t-primary-foreground animate-spin" /> : <Save size={14} />}
                {savingSettings ? t("book.saving") : t("book.save")}
              </button>
            </div>
          </div>
        </aside>
      </section>

      <ConfirmDialog
        open={confirmDeleteOpen}
        title={t("book.deleteBook")}
        message={t("book.confirmDelete")}
        confirmLabel={t("common.delete")}
        cancelLabel={t("common.cancel")}
        variant="danger"
        onConfirm={handleDeleteBook}
        onCancel={() => setConfirmDeleteOpen(false)}
      />

      {selectedArtifact ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-4 backdrop-blur-sm">
          <div className="flex max-h-[88vh] w-full max-w-4xl flex-col rounded-lg border border-border bg-card shadow-xl">
            <div className="flex items-start justify-between gap-4 border-b border-border px-5 py-4">
              <div className="min-w-0">
                <h2 className="truncate text-lg font-semibold text-foreground">{selectedArtifact.title}</h2>
                <p className="mt-1 text-xs text-muted-foreground">{selectedArtifact.jobTitle || t("book.foundationArtifacts")}</p>
              </div>
              <button
                onClick={() => setSelectedArtifact(null)}
                className="rounded-md p-2 text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
                aria-label={t("common.cancel")}
              >
                <X size={18} />
              </button>
            </div>
            {selectedArtifact.backingFiles.length ? (
              <div className="border-b border-border bg-secondary/30 px-5 py-2 font-mono text-xs text-muted-foreground">
                {selectedArtifact.backingFiles.join(" / ")}
              </div>
            ) : null}
            <div className="min-h-0 flex-1 p-5">
              <textarea
                value={artifactEditText}
                onChange={(e) => setArtifactEditText(e.target.value)}
                className="h-[58vh] w-full resize-none rounded-md border border-border bg-background p-4 font-mono text-sm leading-6 text-foreground outline-none focus:border-primary"
                spellCheck={false}
              />
            </div>
            <div className="flex flex-wrap justify-end gap-2 border-t border-border px-5 py-4">
              <button
                onClick={() => setSelectedArtifact(null)}
                className="inline-flex items-center gap-2 rounded-md border border-border bg-secondary px-4 py-2 text-sm font-medium text-foreground transition-colors hover:bg-background"
              >
                <X size={14} />
                {t("common.cancel")}
              </button>
              <button
                onClick={saveFoundationArtifact}
                disabled={savingArtifact}
                className="inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-semibold text-primary-foreground transition-colors hover:opacity-90 disabled:opacity-60"
              >
                {savingArtifact ? <div className="h-4 w-4 rounded-full border-2 border-primary-foreground/25 border-t-primary-foreground animate-spin" /> : <Save size={14} />}
                {savingArtifact ? t("book.saving") : t("book.save")}
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}

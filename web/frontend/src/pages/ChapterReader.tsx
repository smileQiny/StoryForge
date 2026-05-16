import { useEffect, useMemo, useState } from "react";
import { fetchJson, useApi, postApi } from "../hooks/use-api";
import type { Theme } from "../hooks/use-theme";
import type { TFunction } from "../hooks/use-i18n";
import type { ChapterAnalyzeResult } from "../lib/chapter-analysis";
import { summarizeChapterAnalysis } from "../lib/chapter-analysis";
import {
  BrainCircuit,
  BookOpen,
  CheckCircle2,
  ChevronLeft,
  Eye,
  Hash,
  List,
  Pencil,
  Save,
  Type,
  XCircle,
} from "lucide-react";

interface ChapterData {
  readonly chapterNumber: number;
  readonly filename: string;
  readonly content: string;
}

interface Nav {
  toBook: (id: string) => void;
  toDashboard: () => void;
}

function ReaderMetric({ label, value }: { readonly label: string; readonly value: string }) {
  return (
    <div>
      <div className="text-[11px] font-semibold uppercase text-muted-foreground">{label}</div>
      <div className="mt-1 text-sm font-semibold text-foreground">{value}</div>
    </div>
  );
}

export function ChapterReader({ bookId, chapterNumber, nav, theme: _theme, t }: {
  bookId: string;
  chapterNumber: number;
  nav: Nav;
  theme: Theme;
  t: TFunction;
}) {
  const { data, loading, error, refetch } = useApi<ChapterData>(
    `/books/${bookId}/chapters/${chapterNumber}`,
  );
  const [editing, setEditing] = useState(false);
  const [editContent, setEditContent] = useState("");
  const [saving, setSaving] = useState(false);
  const [analysis, setAnalysis] = useState<ChapterAnalyzeResult | null>(null);
  const [analysisLoading, setAnalysisLoading] = useState(false);
  const [analysisError, setAnalysisError] = useState<string | null>(null);

  const parsed = useMemo(() => {
    if (!data) {
      return { title: `Chapter ${chapterNumber}`, body: "", paragraphs: [] as string[] };
    }
    const lines = data.content.split("\n");
    const titleLine = lines.find((line) => line.startsWith("# "));
    const title = titleLine?.replace(/^#\s*/, "") ?? `Chapter ${chapterNumber}`;
    const body = lines.filter((line) => line !== titleLine).join("\n").trim();
    const paragraphs = body.split(/\n\n+/).filter(Boolean);
    return { title, body, paragraphs };
  }, [chapterNumber, data]);
  const currentContent = editing ? editContent : (data?.content ?? "");
  const characterCount = currentContent.length;
  const wordCount = currentContent.trim().split(/\s+/).filter(Boolean).length;
  const analysisSummary = useMemo(() => summarizeChapterAnalysis(analysis), [analysis]);
  const deltaPreview = useMemo(() => {
    if (!analysis?.delta) return "";
    return JSON.stringify(analysis.delta, null, 2);
  }, [analysis]);

  const handleStartEdit = () => {
    if (!data) return;
    setEditContent(data.content);
    setEditing(true);
  };

  useEffect(() => {
    if (!data) return;
    const key = `storyforge:edit-chapter:${bookId}:${chapterNumber}`;
    if (window.sessionStorage.getItem(key) !== "1") return;
    window.sessionStorage.removeItem(key);
    setEditContent(data.content);
    setEditing(true);
  }, [bookId, chapterNumber, data]);

  const handleCancelEdit = () => {
    setEditing(false);
    setEditContent("");
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      await fetchJson(`/books/${bookId}/chapters/${chapterNumber}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ content: editContent }),
      });
      setEditing(false);
      await refetch();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  };

  const handleApprove = async () => {
    await postApi(`/books/${bookId}/chapters/${chapterNumber}/approve`);
    nav.toBook(bookId);
  };

  const handleReject = async () => {
    await postApi(`/books/${bookId}/chapters/${chapterNumber}/reject`);
    nav.toBook(bookId);
  };

  const handleAnalyze = async () => {
    setAnalysisLoading(true);
    setAnalysisError(null);
    try {
      const result = await postApi<ChapterAnalyzeResult>(`/books/${bookId}/chapters/${chapterNumber}/analyze`);
      setAnalysis(result);
    } catch (e) {
      setAnalysisError(e instanceof Error ? e.message : "Analyze failed");
    } finally {
      setAnalysisLoading(false);
    }
  };

  if (loading) {
    return (
      <div className="flex min-h-[50vh] flex-col items-center justify-center gap-3">
        <div className="h-8 w-8 rounded-full border-2 border-primary/20 border-t-primary animate-spin" />
        <span className="text-sm text-muted-foreground">{t("reader.openingManuscript")}</span>
      </div>
    );
  }

  if (error) {
    return <div className="rounded-md border border-destructive/20 bg-destructive/5 p-6 text-sm text-destructive">Error: {error}</div>;
  }

  if (!data) return null;

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
        <button
          onClick={() => nav.toBook(bookId)}
          className="truncate hover:text-foreground transition-colors"
        >
          {bookId}
        </button>
        <span>/</span>
        <span className="inline-flex items-center gap-1 text-foreground">
          <Hash size={12} />
          {chapterNumber}
        </span>
      </nav>

      <section className="border-b border-border/70 pb-6">
        <div className="flex flex-col gap-5 xl:flex-row xl:items-start xl:justify-between">
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-[11px] font-semibold uppercase text-muted-foreground">
              <BookOpen size={14} />
              {t("reader.manuscriptPage")}
            </div>
            <h1 className="mt-2 text-3xl font-semibold text-foreground">{parsed.title}</h1>
            <div className="mt-3 flex flex-wrap gap-4">
              <ReaderMetric label={t("reader.characters")} value={characterCount.toLocaleString()} />
              <ReaderMetric label={t("book.words")} value={wordCount.toLocaleString()} />
              <ReaderMetric label={t("reader.minRead")} value={String(Math.max(1, Math.ceil(characterCount / 1500)))} />
            </div>
          </div>

          <div className="flex flex-wrap gap-2">
            <button
              onClick={() => nav.toBook(bookId)}
              className="inline-flex items-center gap-2 rounded-md border border-border bg-secondary px-4 py-2.5 text-sm font-medium text-foreground transition-colors hover:bg-background"
            >
              <List size={15} />
              {t("reader.backToList")}
            </button>

            <button
              onClick={handleAnalyze}
              disabled={analysisLoading}
              className="inline-flex items-center gap-2 rounded-md border border-border bg-secondary px-4 py-2.5 text-sm font-medium text-foreground transition-colors hover:bg-background disabled:opacity-60"
            >
              {analysisLoading ? <div className="h-4 w-4 rounded-full border-2 border-foreground/20 border-t-foreground animate-spin" /> : <BrainCircuit size={15} />}
              {analysisLoading ? t("reader.analyzing") : t("reader.analyze")}
            </button>

            {editing ? (
              <>
                <button
                  onClick={handleSave}
                  disabled={saving}
                  className="inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2.5 text-sm font-semibold text-primary-foreground transition-colors hover:opacity-90 disabled:opacity-60"
                >
                  {saving ? <div className="h-4 w-4 rounded-full border-2 border-primary-foreground/25 border-t-primary-foreground animate-spin" /> : <Save size={15} />}
                  {saving ? t("book.saving") : t("book.save")}
                </button>
                <button
                  onClick={handleCancelEdit}
                  className="inline-flex items-center gap-2 rounded-md border border-border bg-secondary px-4 py-2.5 text-sm font-medium text-foreground transition-colors hover:bg-background"
                >
                  <Eye size={15} />
                  {t("reader.preview")}
                </button>
              </>
            ) : (
              <button
                onClick={handleStartEdit}
                className="inline-flex items-center gap-2 rounded-md border border-border bg-secondary px-4 py-2.5 text-sm font-medium text-foreground transition-colors hover:bg-background"
              >
                <Pencil size={15} />
                {t("reader.edit")}
              </button>
            )}

            <button
              onClick={handleApprove}
              className="inline-flex items-center gap-2 rounded-md bg-emerald-500/10 px-4 py-2.5 text-sm font-semibold text-emerald-700 transition-colors hover:bg-emerald-500/15 dark:text-emerald-300"
            >
              <CheckCircle2 size={15} />
              {t("reader.approve")}
            </button>
            <button
              onClick={handleReject}
              className="inline-flex items-center gap-2 rounded-md bg-destructive/10 px-4 py-2.5 text-sm font-semibold text-destructive transition-colors hover:bg-destructive/15"
            >
              <XCircle size={15} />
              {t("reader.reject")}
            </button>
          </div>
        </div>
      </section>

      <section className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_240px]">
        <div className="rounded-md border border-border/70 bg-card">
          <div className="border-b border-border/70 px-4 py-3 md:px-6">
            <div className="text-sm font-semibold text-foreground">
              {editing ? t("reader.edit") : t("reader.preview")}
            </div>
          </div>

          <div className="px-4 py-5 md:px-6 md:py-6">
            {editing ? (
              <textarea
                value={editContent}
                onChange={(e) => setEditContent(e.target.value)}
                className="min-h-[70vh] w-full resize-y rounded-md border border-border bg-background px-4 py-4 font-mono text-sm leading-7 text-foreground outline-none focus:border-primary"
                autoFocus
              />
            ) : (
              <article className="mx-auto max-w-3xl space-y-6">
                {parsed.paragraphs.map((paragraph, index) => (
                  <p key={index} className="text-[15px] leading-8 text-foreground">
                    {paragraph}
                  </p>
                ))}
              </article>
            )}
          </div>
        </div>

        <aside className="space-y-6">
          <div className="rounded-md border border-border/70 bg-card p-4 md:p-5">
            <div className="text-sm font-semibold text-foreground">{t("reader.chapterList")}</div>
            <p className="mt-1 text-xs text-muted-foreground">Quick return to the manuscript queue.</p>
            <button
              onClick={() => nav.toBook(bookId)}
              className="mt-4 inline-flex w-full items-center justify-center gap-2 rounded-md border border-border bg-secondary px-4 py-2.5 text-sm font-medium text-foreground transition-colors hover:bg-background"
            >
              <List size={15} />
              {t("reader.backToList")}
            </button>
          </div>

          <div className="rounded-md border border-border/70 bg-card p-4 md:p-5">
            <div className="text-sm font-semibold text-foreground">{t("reader.endOfChapter")}</div>
            <p className="mt-1 text-xs text-muted-foreground">Review the chapter, then approve or send it back for another pass.</p>
            <div className="mt-4 grid gap-2">
              <button
                onClick={handleApprove}
                className="inline-flex items-center justify-center gap-2 rounded-md bg-emerald-500/10 px-4 py-2.5 text-sm font-semibold text-emerald-700 transition-colors hover:bg-emerald-500/15 dark:text-emerald-300"
              >
                <CheckCircle2 size={15} />
                {t("reader.approve")}
              </button>
              <button
                onClick={handleReject}
                className="inline-flex items-center justify-center gap-2 rounded-md bg-destructive/10 px-4 py-2.5 text-sm font-semibold text-destructive transition-colors hover:bg-destructive/15"
              >
                <XCircle size={15} />
                {t("reader.reject")}
              </button>
            </div>
          </div>

          <div className="rounded-md border border-border/70 bg-card p-4 md:p-5">
            <div className="flex items-center justify-between gap-3">
              <div>
                <div className="text-sm font-semibold text-foreground">{t("reader.analysis")}</div>
                <p className="mt-1 text-xs text-muted-foreground">{t("reader.analysisHint")}</p>
              </div>
              <button
                onClick={handleAnalyze}
                disabled={analysisLoading}
                className="inline-flex items-center gap-2 rounded-md border border-border bg-secondary px-3 py-2 text-xs font-medium text-foreground transition-colors hover:bg-background disabled:opacity-60"
              >
                {analysisLoading ? <div className="h-3.5 w-3.5 rounded-full border-2 border-foreground/20 border-t-foreground animate-spin" /> : <BrainCircuit size={14} />}
                {analysisLoading ? t("reader.analyzing") : t("reader.analyze")}
              </button>
            </div>

            {analysisError ? (
              <div className="mt-4 rounded-md border border-destructive/20 bg-destructive/5 px-3 py-2 text-xs text-destructive">
                {analysisError}
              </div>
            ) : null}

            {analysis ? (
              <div className="mt-4 space-y-4">
                <div className="grid grid-cols-2 gap-3">
                  <ReaderMetric label={t("reader.facts")} value={analysisSummary.factCount.toLocaleString()} />
                  <ReaderMetric label={t("reader.newHooks")} value={analysisSummary.hookCount.toLocaleString()} />
                  <ReaderMetric label={t("reader.subplots")} value={analysisSummary.subplotCount.toLocaleString()} />
                  <ReaderMetric label={t("reader.emotionalArcs")} value={analysisSummary.emotionalArcCount.toLocaleString()} />
                  <ReaderMetric label={t("reader.characterMatrix")} value={analysisSummary.matrixCount.toLocaleString()} />
                  <ReaderMetric
                    label={t("reader.stateTransition")}
                    value={analysis.nextState ? t("reader.ready") : t("reader.pending")}
                  />
                </div>

                {analysis.previousSummary ? (
                  <div>
                    <div className="text-[11px] font-semibold uppercase text-muted-foreground">{t("reader.previousSummary")}</div>
                    <p className="mt-1 text-xs leading-6 text-foreground">{analysis.previousSummary}</p>
                  </div>
                ) : null}

                <div>
                  <div className="text-[11px] font-semibold uppercase text-muted-foreground">{t("reader.deltaPreview")}</div>
                  <pre className="mt-2 max-h-64 overflow-auto rounded-md border border-border/70 bg-background px-3 py-3 text-xs leading-6 text-foreground/80">
                    {deltaPreview || "{}"}
                  </pre>
                </div>
              </div>
            ) : !analysisLoading ? (
              <p className="mt-4 text-xs text-muted-foreground">{t("reader.analysisEmpty")}</p>
            ) : null}
          </div>

          <div className="rounded-md border border-border/70 bg-card p-4 md:p-5">
            <div className="text-sm font-semibold text-foreground">{t("reader.characters")}</div>
            <div className="mt-3 space-y-3">
              <ReaderMetric label={t("reader.characters")} value={characterCount.toLocaleString()} />
              <ReaderMetric label={t("book.words")} value={wordCount.toLocaleString()} />
              <ReaderMetric label={t("reader.minRead")} value={String(Math.max(1, Math.ceil(characterCount / 1500)))} />
              <ReaderMetric label={t("chapter.label").replace("{n}", String(chapterNumber))} value={String(chapterNumber)} />
              <div className="pt-2 text-xs text-muted-foreground">
                <div className="inline-flex items-center gap-1">
                  <Type size={12} />
                  {data.filename}
                </div>
              </div>
            </div>
          </div>
        </aside>
      </section>
    </div>
  );
}

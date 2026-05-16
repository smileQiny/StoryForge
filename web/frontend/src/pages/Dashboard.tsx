import { fetchJson, useApi, postApi } from "../hooks/use-api";
import { useEffect, useMemo, useRef, useState } from "react";
import type { SSEMessage } from "../hooks/use-sse";
import type { Theme } from "../hooks/use-theme";
import type { TFunction } from "../hooks/use-i18n";
import { deriveActiveBookIds, shouldRefetchBookCollections } from "../hooks/use-book-activity";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { getBooksFromResponse, type BooksResponse } from "../lib/books";
import { getBookStatusLabel, getBookStatusTone } from "../lib/book-status";
import {
  AlertCircle,
  BarChart2,
  BookOpen,
  Clock,
  Download,
  Flame,
  MoreVertical,
  Plus,
  Settings,
  Trash2,
  Zap,
} from "lucide-react";

interface BookSummary {
  readonly id: string;
  readonly title: string;
  readonly genre: string;
  readonly status: string;
  readonly chaptersWritten?: number;
  readonly language?: string;
  readonly fanficMode?: string;
}

interface Nav {
  toBook: (id: string) => void;
  toAnalytics: (id: string) => void;
  toBookCreate: () => void;
}

function SummaryItem({ label, value }: { readonly label: string; readonly value: string }) {
  return (
    <div className="min-w-0">
      <div className="text-[11px] uppercase text-muted-foreground">{label}</div>
      <div className="mt-1 text-lg font-semibold text-foreground">{value}</div>
    </div>
  );
}

function BookMenu({ bookId, bookTitle, nav, t, onDelete }: {
  readonly bookId: string;
  readonly bookTitle: string;
  readonly nav: Nav;
  readonly t: TFunction;
  readonly onDelete: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const handleClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [open]);

  const handleDelete = async () => {
    setConfirmDelete(false);
    setOpen(false);
    await fetchJson(`/books/${bookId}`, { method: "DELETE" });
    onDelete();
  };

  return (
    <div ref={menuRef} className="relative">
      <button
        onClick={() => setOpen((prev) => !prev)}
        className="flex h-10 w-10 items-center justify-center rounded-md border border-border bg-background text-muted-foreground hover:text-foreground transition-colors"
        aria-label={t("book.curate")}
      >
        <MoreVertical size={16} />
      </button>
      {open && (
        <div className="absolute right-0 top-full z-50 mt-1 w-44 overflow-hidden rounded-md border border-border bg-card py-1 shadow-lg">
          <button
            onClick={() => {
              setOpen(false);
              nav.toBook(bookId);
            }}
            className="flex w-full items-center gap-2 px-3 py-2 text-sm text-foreground hover:bg-secondary transition-colors"
          >
            <Settings size={14} className="text-muted-foreground" />
            {t("book.settings")}
          </button>
          <a
            href={`/api/books/${bookId}/export?format=txt`}
            download
            onClick={() => setOpen(false)}
            className="flex w-full items-center gap-2 px-3 py-2 text-sm text-foreground hover:bg-secondary transition-colors"
          >
            <Download size={14} className="text-muted-foreground" />
            {t("book.export")}
          </a>
          <div className="my-1 border-t border-border/70" />
          <button
            onClick={() => {
              setOpen(false);
              setConfirmDelete(true);
            }}
            className="flex w-full items-center gap-2 px-3 py-2 text-sm text-destructive hover:bg-destructive/10 transition-colors"
          >
            <Trash2 size={14} />
            {t("book.deleteBook")}
          </button>
        </div>
      )}
      <ConfirmDialog
        open={confirmDelete}
        title={t("book.deleteBook")}
        message={`${t("book.confirmDelete")}\n\n"${bookTitle}"`}
        confirmLabel={t("common.delete")}
        cancelLabel={t("common.cancel")}
        variant="danger"
        onConfirm={handleDelete}
        onCancel={() => setConfirmDelete(false)}
      />
    </div>
  );
}

export function Dashboard({ nav, sse, theme: _theme, t }: {
  nav: Nav;
  sse: { messages: ReadonlyArray<SSEMessage> };
  theme: Theme;
  t: TFunction;
}) {
  const { data, loading, error, refetch } = useApi<BooksResponse<BookSummary>>("/books");
  const writingBooks = useMemo(() => deriveActiveBookIds(sse.messages), [sse.messages]);
  const logEvents = useMemo(
    () => sse.messages.filter((message) => message.event === "log").slice(-8),
    [sse.messages],
  );
  const progressEvent = useMemo(
    () => [...sse.messages].reverse().find((message) => message.event === "llm:progress"),
    [sse.messages],
  );

  useEffect(() => {
    const recent = sse.messages.at(-1);
    if (!recent) return;
    if (shouldRefetchBookCollections(recent)) {
      refetch();
    }
  }, [refetch, sse.messages]);

  const books = getBooksFromResponse(data);
  const totalChapters = useMemo(
    () => books.reduce((sum, book) => sum + (book.chaptersWritten ?? 0), 0),
    [books],
  );
  const activeBooks = useMemo(
    () => books.filter((book) => book.status === "active").length,
    [books],
  );

  if (loading) {
    return (
      <div className="flex min-h-[50vh] flex-col items-center justify-center gap-3">
        <div className="h-8 w-8 rounded-full border-2 border-primary/20 border-t-primary animate-spin" />
        <span className="text-sm text-muted-foreground">Gathering manuscripts...</span>
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-md border border-destructive/20 bg-destructive/5 px-6 py-10 text-center">
        <AlertCircle className="mx-auto mb-3 text-destructive" size={28} />
        <h2 className="text-base font-semibold text-destructive">Failed to load library</h2>
        <p className="mt-1 text-sm text-muted-foreground">{error}</p>
      </div>
    );
  }

  if (!books.length) {
    return (
      <div className="flex min-h-[60vh] flex-col items-center justify-center text-center">
        <div className="flex h-16 w-16 items-center justify-center rounded-md border border-border bg-secondary text-primary">
          <BookOpen size={30} />
        </div>
        <h2 className="mt-5 text-2xl font-semibold text-foreground">{t("dash.noBooks")}</h2>
        <p className="mt-2 max-w-sm text-sm text-muted-foreground">{t("dash.createFirst")}</p>
        <button
          onClick={nav.toBookCreate}
          className="mt-6 inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2.5 text-sm font-semibold text-primary-foreground transition-colors hover:opacity-90"
        >
          <Plus size={16} />
          {t("nav.newBook")}
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <section className="border-b border-border/70 pb-5">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div className="min-w-0">
            <p className="text-[11px] font-semibold uppercase text-muted-foreground">Studio</p>
            <h1 className="mt-1 text-3xl font-semibold text-foreground">{t("dash.title")}</h1>
            <p className="mt-2 max-w-2xl text-sm text-muted-foreground">{t("dash.subtitle")}</p>
          </div>
          <button
            onClick={nav.toBookCreate}
            className="inline-flex items-center justify-center gap-2 rounded-md bg-primary px-4 py-2.5 text-sm font-semibold text-primary-foreground transition-colors hover:opacity-90"
          >
            <Plus size={16} />
            {t("nav.newBook")}
          </button>
        </div>
      </section>

      <section className="grid gap-4 border-b border-border/70 pb-5 sm:grid-cols-2 xl:grid-cols-4">
        <SummaryItem label={t("nav.books")} value={String(books.length)} />
        <SummaryItem label={t("dash.chapters")} value={String(totalChapters)} />
        <SummaryItem label={t("book.statusActive")} value={String(activeBooks)} />
        <SummaryItem label={t("dash.writingProgress")} value={String(writingBooks.size)} />
      </section>

      <section className="overflow-hidden rounded-md border border-border/70 bg-card">
        <div className="grid grid-cols-[minmax(0,1.8fr)_minmax(0,1fr)_auto] gap-4 border-b border-border/70 px-4 py-3 text-[11px] font-semibold uppercase text-muted-foreground md:px-5">
          <div>{t("nav.books")}</div>
          <div className="hidden md:block">{t("dash.writingProgress")}</div>
          <div className="text-right">{t("book.curate")}</div>
        </div>

        <div className="divide-y divide-border/70">
          {books.map((book) => {
            const isWriting = writingBooks.has(book.id);
            return (
              <div key={book.id} className="grid gap-4 px-4 py-4 md:grid-cols-[minmax(0,1.8fr)_minmax(0,1fr)_auto] md:px-5">
                <div className="min-w-0">
                  <button
                    onClick={() => nav.toBook(book.id)}
                    className="truncate text-left text-base font-semibold text-foreground hover:text-primary transition-colors"
                  >
                    {book.title}
                  </button>
                  <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                    <span className="rounded-sm bg-secondary px-2 py-1">{book.genre}</span>
                    <span className={`rounded-sm px-2 py-1 ${getBookStatusTone(book.status)}`}>
                      {getBookStatusLabel(book.status, t)}
                    </span>
                    <span className="inline-flex items-center gap-1">
                      <Clock size={13} />
                      {book.chaptersWritten ?? 0} {t("dash.chapters")}
                    </span>
                    {book.language === "en" && (
                      <span className="rounded-sm border border-primary/20 px-2 py-1 text-primary">EN</span>
                    )}
                    {book.fanficMode && (
                      <span className="rounded-sm bg-secondary px-2 py-1">{book.fanficMode}</span>
                    )}
                  </div>
                </div>

                <div className="flex min-w-0 flex-col justify-center gap-2">
                  <button
                    onClick={() => postApi(`/books/${book.id}/write-next`)}
                    disabled={isWriting}
                    className={`inline-flex items-center justify-center gap-2 rounded-md px-4 py-2 text-sm font-semibold transition-colors ${
                      isWriting
                        ? "bg-primary/15 text-primary"
                        : "bg-secondary text-foreground hover:bg-primary hover:text-primary-foreground"
                    }`}
                  >
                    {isWriting ? (
                      <>
                        <div className="h-4 w-4 rounded-full border-2 border-primary/20 border-t-primary animate-spin" />
                        {t("dash.writing")}
                      </>
                    ) : (
                      <>
                        <Zap size={16} />
                        {t("dash.writeNext")}
                      </>
                    )}
                  </button>
                  <button
                    onClick={() => nav.toAnalytics(book.id)}
                    className="inline-flex items-center justify-center gap-2 rounded-md border border-border bg-background px-4 py-2 text-sm font-medium text-foreground transition-colors hover:bg-secondary"
                  >
                    <BarChart2 size={15} />
                    {t("dash.stats")}
                  </button>
                </div>

                <div className="flex items-start justify-end">
                  <BookMenu
                    bookId={book.id}
                    bookTitle={book.title}
                    nav={nav}
                    t={t}
                    onDelete={() => refetch()}
                  />
                </div>
              </div>
            );
          })}
        </div>
      </section>

      {writingBooks.size > 0 && logEvents.length > 0 && (
        <section className="rounded-md border border-border/70 bg-card px-4 py-4 md:px-5">
          <div className="flex flex-col gap-3 border-b border-border/70 pb-4 md:flex-row md:items-center md:justify-between">
            <div className="flex items-center gap-3">
              <div className="flex h-9 w-9 items-center justify-center rounded-md bg-primary/10 text-primary">
                <Flame size={16} />
              </div>
              <div>
                <h2 className="text-sm font-semibold text-foreground">{t("dash.writingProgress")}</h2>
                <p className="text-xs text-muted-foreground">Live pipeline activity</p>
              </div>
            </div>
            {progressEvent && (
              <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                <span>
                  {Math.round(((progressEvent.data as { elapsedMs?: number })?.elapsedMs ?? 0) / 1000)}s
                </span>
                <span>
                  {((progressEvent.data as { totalChars?: number })?.totalChars ?? 0).toLocaleString()} chars
                </span>
              </div>
            )}
          </div>
          <div className="mt-4 space-y-2 font-mono text-xs">
            {logEvents.map((message, index) => {
              const data = message.data as { tag?: string; message?: string };
              return (
                <div key={`${message.timestamp}-${index}`} className="grid grid-cols-[auto_1fr] gap-3 text-muted-foreground">
                  <span className="text-primary/80">[{data.tag ?? "log"}]</span>
                  <span>{data.message}</span>
                </div>
              );
            })}
          </div>
        </section>
      )}
    </div>
  );
}

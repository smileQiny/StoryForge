import { useEffect } from "react";
import { useApi } from "../hooks/use-api";
import type { SSEMessage } from "../hooks/use-sse";
import { shouldRefetchBookCollections, shouldRefetchDaemonStatus } from "../hooks/use-book-activity";
import type { TFunction } from "../hooks/use-i18n";
import { getBooksFromResponse, type BooksResponse } from "../lib/books";
import {
  Book,
  LayoutGrid,
  Settings,
  Terminal,
  Plus,
  ScrollText,
  Boxes,
  Zap,
  Wand2,
  FileInput,
  TrendingUp,
  Stethoscope,
} from "lucide-react";

interface BookSummary {
  readonly id: string;
  readonly title: string;
  readonly genre: string;
  readonly status: string;
  readonly chaptersWritten?: number;
}

interface Nav {
  toDashboard: () => void;
  toBook: (id: string) => void;
  toBookCreate: () => void;
  toConfig: () => void;
  toDaemon: () => void;
  toLogs: () => void;
  toGenres: () => void;
  toStyle: () => void;
  toImport: () => void;
  toRadar: () => void;
  toDoctor: () => void;
  toOps: () => void;
}

export function Sidebar({ nav, activePage, sse, t }: {
  nav: Nav;
  activePage: string;
  sse: { messages: ReadonlyArray<SSEMessage>; connected?: boolean };
  t: TFunction;
}) {
  const { data, refetch: refetchBooks } = useApi<BooksResponse<BookSummary>>("/books");
  const { data: daemon, refetch: refetchDaemon } = useApi<{ running: boolean }>("/daemon");
  const books = getBooksFromResponse(data);
  const agentOnline = Boolean(daemon?.running || sse.connected);

  useEffect(() => {
    const recent = sse.messages.at(-1);
    if (!recent) return;
    if (shouldRefetchBookCollections(recent)) {
      refetchBooks();
    }
    if (shouldRefetchDaemonStatus(recent)) {
      refetchDaemon();
    }
  }, [refetchBooks, refetchDaemon, sse.messages]);

  useEffect(() => {
    const id = window.setInterval(() => {
      refetchDaemon();
    }, 5000);
    return () => window.clearInterval(id);
  }, [refetchDaemon]);

  return (
    <aside className="w-[272px] shrink-0 border-r border-border/70 bg-background flex flex-col h-full overflow-hidden select-none">
      <div className="px-5 py-5 border-b border-border/70">
        <button
          onClick={nav.toDashboard}
          className="group flex w-full items-center gap-3 rounded-md px-1 py-1 text-left"
        >
          <div className="flex h-9 w-9 items-center justify-center rounded-md border border-primary/20 bg-primary/10 text-primary">
            <ScrollText size={17} />
          </div>
          <div className="min-w-0 flex-1">
            <span className="block truncate text-sm font-semibold text-foreground">Storyforge Studio</span>
            <span className="block truncate text-xs text-muted-foreground">Novel workspace</span>
          </div>
        </button>
      </div>

      <div className="flex-1 overflow-y-auto px-3 py-4 space-y-6">
        <div className="space-y-1">
          <SidebarItem
            label={t("dash.title")}
            icon={<LayoutGrid size={16} />}
            active={activePage === "dashboard"}
            onClick={nav.toDashboard}
          />
        </div>

        <div>
          <div className="mb-2 flex items-center justify-between px-2">
            <span className="text-[11px] font-semibold uppercase text-muted-foreground">
              {t("nav.books")}
            </span>
            <button
              onClick={nav.toBookCreate}
              className="flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground hover:bg-secondary hover:text-foreground transition-colors"
              title={t("nav.newBook")}
              aria-label={t("nav.newBook")}
            >
              <Plus size={14} />
            </button>
          </div>

          <div className="space-y-1">
            {books.map((book) => (
              <button
                key={book.id}
                onClick={() => nav.toBook(book.id)}
                className={`w-full group flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors ${
                  activePage === `book:${book.id}`
                    ? "bg-primary/10 text-foreground"
                    : "text-muted-foreground hover:bg-secondary hover:text-foreground"
                }`}
              >
                <Book
                  size={16}
                  className={activePage === `book:${book.id}` ? "text-primary" : "text-muted-foreground group-hover:text-foreground"}
                />
                <span className="truncate flex-1 text-left font-medium">{book.title}</span>
                {(book.chaptersWritten ?? 0) > 0 && (
                  <span className="rounded-sm bg-secondary px-1.5 py-0.5 text-[10px] text-muted-foreground">
                    {book.chaptersWritten ?? 0}
                  </span>
                )}
              </button>
            ))}

            {books.length === 0 && (
              <div className="rounded-md border border-dashed border-border px-3 py-5 text-center text-xs text-muted-foreground">
                {t("dash.noBooks")}
              </div>
            )}
          </div>
        </div>

        <div>
          <div className="mb-2 px-2">
            <span className="text-[11px] font-semibold uppercase text-muted-foreground">
              {t("nav.system")}
            </span>
          </div>
          <div className="space-y-1">
            <SidebarItem
              label={t("create.genre")}
              icon={<Boxes size={16} />}
              active={activePage === "genres"}
              onClick={nav.toGenres}
            />
            <SidebarItem
              label={t("nav.config")}
              icon={<Settings size={16} />}
              active={activePage === "config"}
              onClick={nav.toConfig}
            />
            <SidebarItem
              label={t("nav.daemon")}
              icon={<Zap size={16} />}
              active={activePage === "daemon"}
              onClick={nav.toDaemon}
              badge={daemon?.running ? t("nav.running") : undefined}
              badgeColor={daemon?.running ? "bg-emerald-500/10 text-emerald-500" : "bg-muted text-muted-foreground"}
            />
            <SidebarItem
              label={t("nav.logs")}
              icon={<Terminal size={16} />}
              active={activePage === "logs"}
              onClick={nav.toLogs}
            />
          </div>
        </div>

        <div>
          <div className="mb-2 px-2">
            <span className="text-[11px] font-semibold uppercase text-muted-foreground">
              {t("nav.tools")}
            </span>
          </div>
          <div className="space-y-1">
            <SidebarItem
              label={t("nav.style")}
              icon={<Wand2 size={16} />}
              active={activePage === "style"}
              onClick={nav.toStyle}
            />
            <SidebarItem
              label={t("nav.import")}
              icon={<FileInput size={16} />}
              active={activePage === "import"}
              onClick={nav.toImport}
            />
            <SidebarItem
              label={t("nav.radar")}
              icon={<TrendingUp size={16} />}
              active={activePage === "radar"}
              onClick={nav.toRadar}
            />
            <SidebarItem
              label={t("nav.doctor")}
              icon={<Stethoscope size={16} />}
              active={activePage === "doctor"}
              onClick={nav.toDoctor}
            />
            <SidebarItem
              label={t("nav.ops")}
              icon={<Terminal size={16} />}
              active={activePage === "ops"}
              onClick={nav.toOps}
            />
          </div>
        </div>
      </div>

      <div className="border-t border-border/70 px-5 py-4">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <div className={`h-2 w-2 rounded-full ${agentOnline ? "bg-emerald-500" : "bg-muted-foreground/40"}`} />
          <span className="font-medium">
            {agentOnline ? t("nav.agentOnline") : t("nav.agentOffline")}
          </span>
        </div>
      </div>
    </aside>
  );
}

function SidebarItem({ label, icon, active, onClick, badge, badgeColor }: {
  label: string;
  icon: React.ReactNode;
  active: boolean;
  onClick: () => void;
  badge?: string;
  badgeColor?: string;
}) {
  return (
    <button
      onClick={onClick}
      className={`w-full group flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors ${
        active
          ? "bg-secondary text-foreground"
          : "text-muted-foreground hover:bg-secondary hover:text-foreground"
      }`}
    >
      <span className={`transition-colors ${active ? "text-primary" : "text-muted-foreground group-hover:text-foreground"}`}>
        {icon}
      </span>
      <span className="flex-1 text-left font-medium">{label}</span>
      {badge && (
        <span className={`rounded-sm px-1.5 py-0.5 text-[9px] font-semibold uppercase ${badgeColor}`}>
          {badge}
        </span>
      )}
    </button>
  );
}

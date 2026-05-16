import { Suspense, lazy, useState, useEffect, useMemo } from "react";
import { Sidebar } from "./components/Sidebar";
import { LanguageSelector } from "./pages/LanguageSelector";
import { useSSE } from "./hooks/use-sse";
import { useTheme } from "./hooks/use-theme";
import { useI18n } from "./hooks/use-i18n";
import { postApi, useApi } from "./hooks/use-api";
import { Sun, Moon, MessageSquare, Wifi, WifiOff } from "lucide-react";

const Dashboard = lazy(() => import("./pages/Dashboard").then((module) => ({ default: module.Dashboard })));
const BookDetail = lazy(() => import("./pages/BookDetail").then((module) => ({ default: module.BookDetail })));
const BookCreate = lazy(() => import("./pages/BookCreate").then((module) => ({ default: module.BookCreate })));
const ChapterReader = lazy(() => import("./pages/ChapterReader").then((module) => ({ default: module.ChapterReader })));
const Analytics = lazy(() => import("./pages/Analytics").then((module) => ({ default: module.Analytics })));
const ConfigView = lazy(() => import("./pages/ConfigView").then((module) => ({ default: module.ConfigView })));
const TruthFiles = lazy(() => import("./pages/TruthFiles").then((module) => ({ default: module.TruthFiles })));
const DaemonControl = lazy(() => import("./pages/DaemonControl").then((module) => ({ default: module.DaemonControl })));
const LogViewer = lazy(() => import("./pages/LogViewer").then((module) => ({ default: module.LogViewer })));
const GenreManager = lazy(() => import("./pages/GenreManager").then((module) => ({ default: module.GenreManager })));
const StyleManager = lazy(() => import("./pages/StyleManager").then((module) => ({ default: module.StyleManager })));
const ImportManager = lazy(() => import("./pages/ImportManager").then((module) => ({ default: module.ImportManager })));
const RadarView = lazy(() => import("./pages/RadarView").then((module) => ({ default: module.RadarView })));
const DoctorView = lazy(() => import("./pages/DoctorView").then((module) => ({ default: module.DoctorView })));
const OpsView = lazy(() => import("./pages/OpsView").then((module) => ({ default: module.OpsView })));
const ChatPanel = lazy(() => import("./components/ChatBar").then((module) => ({ default: module.ChatPanel })));

export type Route =
  | { page: "dashboard" }
  | { page: "book"; bookId: string }
  | { page: "book-create" }
  | { page: "chapter"; bookId: string; chapterNumber: number }
  | { page: "analytics"; bookId: string }
  | { page: "config" }
  | { page: "truth"; bookId: string }
  | { page: "daemon" }
  | { page: "logs" }
  | { page: "genres" }
  | { page: "style" }
  | { page: "import" }
  | { page: "radar" }
  | { page: "doctor" }
  | { page: "ops" };

export function deriveActiveBookId(route: Route): string | undefined {
  return route.page === "book" || route.page === "chapter" || route.page === "truth" || route.page === "analytics"
    ? route.bookId
    : undefined;
}

function getRouteLabel(route: Route, t: ReturnType<typeof useI18n>["t"]): string {
  switch (route.page) {
    case "dashboard":
      return t("dash.title");
    case "book-create":
      return t("create.title");
    case "config":
      return t("config.title");
    case "daemon":
      return t("nav.daemon");
    case "logs":
      return t("nav.logs");
    case "genres":
      return t("create.genre");
    case "style":
      return t("style.title");
    case "import":
      return t("import.title");
    case "radar":
      return t("radar.title");
    case "doctor":
      return t("doctor.title");
    case "analytics":
      return t("analytics.title");
    case "truth":
      return t("truth.title");
    case "chapter":
      return t("reader.chapterList");
    case "ops":
      return t("nav.ops");
    case "book":
      return t("bread.books");
  }
}

function RouteFallback() {
  return (
    <div className="flex min-h-[240px] items-center justify-center">
      <div className="h-10 w-10 rounded-full border-4 border-primary/20 border-t-primary animate-spin" />
    </div>
  );
}

export function App() {
  const [route, setRoute] = useState<Route>({ page: "dashboard" });
  const sse = useSSE();
  const { theme, setTheme } = useTheme();
  const { t } = useI18n();
  const { data: project, refetch: refetchProject } = useApi<{ language: string; languageExplicit: boolean }>("/project");
  const [showLanguageSelector, setShowLanguageSelector] = useState(false);
  const [ready, setReady] = useState(false);
  const [chatOpen, setChatOpen] = useState(false);

  const isDark = theme === "dark";

  useEffect(() => {
    document.documentElement.classList.toggle("dark", isDark);
  }, [isDark]);

  useEffect(() => {
    if (project) {
      if (!project.languageExplicit) {
        setShowLanguageSelector(true);
      }
      setReady(true);
    }
  }, [project]);

  const nav = useMemo(() => ({
    toDashboard: () => setRoute({ page: "dashboard" }),
    toBook: (bookId: string) => setRoute({ page: "book", bookId }),
    toBookCreate: () => setRoute({ page: "book-create" }),
    toChapter: (bookId: string, chapterNumber: number) =>
      setRoute({ page: "chapter", bookId, chapterNumber }),
    toAnalytics: (bookId: string) => setRoute({ page: "analytics", bookId }),
    toConfig: () => setRoute({ page: "config" }),
    toTruth: (bookId: string) => setRoute({ page: "truth", bookId }),
    toDaemon: () => setRoute({ page: "daemon" }),
    toLogs: () => setRoute({ page: "logs" }),
    toGenres: () => setRoute({ page: "genres" }),
    toStyle: () => setRoute({ page: "style" }),
    toImport: () => setRoute({ page: "import" }),
    toRadar: () => setRoute({ page: "radar" }),
    toDoctor: () => setRoute({ page: "doctor" }),
    toOps: () => setRoute({ page: "ops" }),
  }), []);

  const activeBookId = deriveActiveBookId(route);
  const activePage =
    activeBookId
      ? `book:${activeBookId}`
      : route.page;
  const routeLabel = getRouteLabel(route, t);

  if (!ready) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center">
        <div className="w-12 h-12 border-4 border-primary/20 border-t-primary rounded-full animate-spin" />
      </div>
    );
  }

  if (showLanguageSelector) {
    return (
      <LanguageSelector
        onSelect={async (lang) => {
          await postApi("/project/language", { language: lang });
          setShowLanguageSelector(false);
          refetchProject();
        }}
      />
    );
  }

  return (
    <div className="h-dvh bg-background text-foreground flex overflow-hidden font-sans">
      <Sidebar nav={nav} activePage={activePage} sse={sse} t={t} />

      <div className="flex-1 flex flex-col min-w-0 bg-background">
        <header className="h-14 shrink-0 flex items-center justify-between gap-4 px-4 md:px-6 border-b border-border/70 bg-background/95">
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold text-foreground">{routeLabel}</p>
            <p className="truncate text-xs text-muted-foreground">Storyforge Studio</p>
          </div>

          <div className="flex items-center gap-2">
            <div
              className={`hidden sm:flex items-center gap-2 rounded-md border px-2.5 py-1.5 text-xs font-medium ${
                sse.connected
                  ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"
                  : "border-border bg-secondary text-muted-foreground"
              }`}
              aria-live="polite"
            >
              {sse.connected ? <Wifi size={14} /> : <WifiOff size={14} />}
              {sse.connected ? t("nav.connected") : t("nav.disconnected")}
            </div>
            <button
              onClick={() => setTheme(isDark ? "light" : "dark")}
              className="w-9 h-9 flex items-center justify-center rounded-md border border-border bg-secondary text-muted-foreground hover:text-foreground transition-colors"
              title={isDark ? "Switch to Light Mode" : "Switch to Dark Mode"}
              aria-label={isDark ? "Switch to light mode" : "Switch to dark mode"}
            >
              {isDark ? <Sun size={16} /> : <Moon size={16} />}
            </button>

            <button
              onClick={() => setChatOpen((prev) => !prev)}
              className={`w-9 h-9 flex items-center justify-center rounded-md border transition-colors ${
                chatOpen
                  ? "border-primary bg-primary text-primary-foreground"
                  : "border-border bg-secondary text-muted-foreground hover:text-foreground"
              }`}
              title="Toggle AI Assistant"
              aria-label="Toggle AI assistant"
              aria-pressed={chatOpen}
            >
              <MessageSquare size={16} />
            </button>
          </div>
        </header>

        <main className="flex-1 overflow-y-auto scroll-smooth">
          <div className="w-full max-w-6xl mx-auto px-4 py-8 md:px-8 lg:py-10 fade-in">
            <Suspense fallback={<RouteFallback />}>
              {route.page === "dashboard" && <Dashboard nav={nav} sse={sse} theme={theme} t={t} />}
              {route.page === "book" && <BookDetail bookId={route.bookId} nav={nav} theme={theme} t={t} sse={sse} />}
              {route.page === "book-create" && <BookCreate nav={nav} theme={theme} t={t} />}
              {route.page === "chapter" && <ChapterReader bookId={route.bookId} chapterNumber={route.chapterNumber} nav={nav} theme={theme} t={t} />}
              {route.page === "analytics" && <Analytics bookId={route.bookId} nav={nav} theme={theme} t={t} />}
              {route.page === "config" && <ConfigView nav={nav} theme={theme} t={t} />}
              {route.page === "truth" && <TruthFiles bookId={route.bookId} nav={nav} theme={theme} t={t} />}
              {route.page === "daemon" && <DaemonControl nav={nav} theme={theme} t={t} sse={sse} />}
              {route.page === "logs" && <LogViewer nav={nav} theme={theme} t={t} />}
              {route.page === "genres" && <GenreManager nav={nav} theme={theme} t={t} />}
              {route.page === "style" && <StyleManager nav={nav} theme={theme} t={t} />}
              {route.page === "import" && <ImportManager nav={nav} theme={theme} t={t} />}
              {route.page === "radar" && <RadarView nav={nav} theme={theme} t={t} />}
              {route.page === "doctor" && <DoctorView nav={nav} theme={theme} t={t} />}
              {route.page === "ops" && <OpsView nav={nav} theme={theme} t={t} />}
            </Suspense>
          </div>
        </main>
      </div>

      {chatOpen ? (
        <Suspense fallback={null}>
          <ChatPanel
            open={chatOpen}
            onClose={() => setChatOpen(false)}
            t={t}
            sse={sse}
            activeBookId={activeBookId}
          />
        </Suspense>
      ) : null}
    </div>
  );
}

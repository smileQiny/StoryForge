package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

func Run(args []string) error {
	flags := flag.NewFlagSet("storyforge tui", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	addr := flags.String("addr", DefaultAddr, "StoryForge server URL")
	noClear := flags.Bool("no-clear", false, "disable terminal screen clearing")
	if err := flags.Parse(args); err != nil {
		return err
	}

	client, err := NewClient(*addr)
	if err != nil {
		return err
	}
	app := &App{
		client:  client,
		addr:    *addr,
		noClear: *noClear,
		in:      bufio.NewReader(os.Stdin),
	}
	return app.Run(context.Background())
}

type App struct {
	client  *Client
	addr    string
	noClear bool
	in      *bufio.Reader

	lastBookID string
	message    string
}

func (a *App) Run(ctx context.Context) error {
	for {
		snapshot := a.load(ctx)
		a.render(snapshot)
		cmd, err := a.prompt()
		if err != nil {
			return err
		}
		if a.handle(ctx, cmd, snapshot) {
			return nil
		}
	}
}

type snapshot struct {
	healthOK bool
	books    []BookSummary
	daemon   *DaemonStatus
	err      error
}

func (a *App) load(ctx context.Context) snapshot {
	var s snapshot
	s.healthOK = a.client.Health(ctx) == nil
	books, err := a.client.ListBooks(ctx)
	if err != nil {
		s.err = err
		return s
	}
	sort.Slice(books, func(i, j int) bool {
		return books[i].ID < books[j].ID
	})
	s.books = books
	daemon, err := a.client.Daemon(ctx)
	if err != nil {
		s.err = err
		return s
	}
	s.daemon = daemon
	return s
}

func (a *App) render(s snapshot) {
	if !a.noClear {
		fmt.Print("\033[2J\033[H")
	}
	fmt.Println("StoryForge TUI")
	fmt.Println(strings.Repeat("=", 64))
	fmt.Printf("Server: %s  Health: %s", a.addr, okText(s.healthOK))
	if s.daemon != nil {
		fmt.Printf("  Daemon: %s", runningText(s.daemon.Running))
	}
	fmt.Println()
	if a.message != "" {
		fmt.Printf("Message: %s\n", a.message)
	}
	if s.err != nil {
		fmt.Printf("Error: %v\n", s.err)
	}
	fmt.Println()
	fmt.Println("Books")
	fmt.Println(strings.Repeat("-", 64))
	if len(s.books) == 0 {
		fmt.Println("No books yet.")
	} else {
		for i, book := range s.books {
			marker := " "
			if book.ID == a.lastBookID {
				marker = "*"
			}
			fmt.Printf("%s %2d. %-24s %-14s %s/%d ch %d words\n",
				marker, i+1, trim(book.Title, 24), book.Status, book.Language, book.TargetChapters, book.ChapterWordCount)
			fmt.Printf("      id: %s  genre: %s\n", book.ID, book.Genre)
		}
	}
	fmt.Println()
	if s.daemon != nil {
		fmt.Println("Daemon Summary")
		fmt.Println(strings.Repeat("-", 64))
		fmt.Printf("books total=%d active=%d  runs running=%d queued=%d succeeded=%d failed=%d\n",
			s.daemon.Summary.BooksTotal,
			s.daemon.Summary.BooksActive,
			s.daemon.Summary.RunsRunning,
			s.daemon.Summary.RunsQueued,
			s.daemon.Summary.RunsSucceeded,
			s.daemon.Summary.RunsFailed,
		)
		fmt.Println()
	}
	fmt.Println("Commands")
	fmt.Println("  r | q | help")
	fmt.Println("  new | book <book> | delete <book> | chapters <book>")
	fmt.Println("  read <book> <n> | edit <book> <n> | approve/reject/analyze <book> <n>")
	fmt.Println("  write <book> | draft <book> | audit/revise/rewrite <book> <n>")
	fmt.Println("  truth <book> [file] | truth-edit <book> <file> | export <book> [txt]")
	fmt.Println("  daemon start|stop|poll|toggle | logs [limit] | doctor | config | models | profile-test <name> | genres [language]")
	fmt.Println("Book can be a list number or book id.")
}

func (a *App) prompt() (string, error) {
	fmt.Print("> ")
	line, err := a.in.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (a *App) handle(ctx context.Context, cmd string, s snapshot) bool {
	a.message = ""
	fields := strings.Fields(cmd)
	if len(fields) == 0 || fields[0] == "r" || fields[0] == "refresh" {
		return false
	}
	switch fields[0] {
	case "q", "quit", "exit":
		return true
	case "help", "h", "?":
		a.renderHelp()
	case "new":
		a.createBook(ctx)
	case "book":
		bookID, ok := a.resolveBook(fields[1:], s.books)
		if !ok {
			return false
		}
		a.lastBookID = bookID
		a.renderBook(ctx, bookID)
	case "delete", "rm":
		bookID, ok := a.resolveBook(fields[1:], s.books)
		if !ok {
			return false
		}
		if !a.confirm("delete book " + bookID + "? type yes: ") {
			a.message = "delete cancelled"
			return false
		}
		if err := a.client.DeleteBook(ctx, bookID); err != nil {
			a.message = "delete failed: " + err.Error()
			return false
		}
		a.message = "book deleted: " + bookID
	case "d", "daemon":
		a.handleDaemon(ctx, fields[1:], s)
	case "c", "chapters":
		bookID, ok := a.resolveBook(fields[1:], s.books)
		if !ok {
			return false
		}
		a.lastBookID = bookID
		a.renderChapters(ctx, bookID)
	case "read":
		bookID, chapter, ok := a.resolveBookChapter(fields[1:], s.books)
		if !ok {
			return false
		}
		a.lastBookID = bookID
		a.renderChapter(ctx, bookID, chapter)
	case "edit":
		bookID, chapter, ok := a.resolveBookChapter(fields[1:], s.books)
		if !ok {
			return false
		}
		a.lastBookID = bookID
		a.editChapter(ctx, bookID, chapter)
	case "approve":
		bookID, chapter, ok := a.resolveBookChapter(fields[1:], s.books)
		if !ok {
			return false
		}
		if err := a.client.ApproveChapter(ctx, bookID, chapter); err != nil {
			a.message = "approve failed: " + err.Error()
			return false
		}
		a.message = fmt.Sprintf("chapter approved: %s #%d", bookID, chapter)
	case "reject":
		bookID, chapter, ok := a.resolveBookChapter(fields[1:], s.books)
		if !ok {
			return false
		}
		reason := strings.Join(fields[3:], " ")
		if reason == "" {
			reason = a.promptDefault("reject reason", "needs revision")
		}
		if _, err := a.client.RejectChapter(ctx, bookID, chapter, reason); err != nil {
			a.message = "reject failed: " + err.Error()
			return false
		}
		a.message = fmt.Sprintf("chapter rejected: %s #%d", bookID, chapter)
	case "analyze":
		bookID, chapter, ok := a.resolveBookChapter(fields[1:], s.books)
		if !ok {
			return false
		}
		result, err := a.client.AnalyzeChapter(ctx, bookID, chapter)
		if err != nil {
			a.message = "analyze failed: " + err.Error()
			return false
		}
		a.renderJSON("Chapter Analysis", result)
	case "w", "write", "write-next":
		bookID, ok := a.resolveBook(fields[1:], s.books)
		if !ok {
			return false
		}
		a.lastBookID = bookID
		accepted, err := a.client.WriteNext(ctx, bookID)
		if err != nil {
			a.message = "write-next failed: " + err.Error()
			return false
		}
		a.message = "write-next accepted"
		if accepted.RunID != "" {
			a.message += ": " + accepted.RunID
		}
	case "draft":
		bookID, ok := a.resolveBook(fields[1:], s.books)
		if !ok {
			return false
		}
		a.lastBookID = bookID
		accepted, err := a.client.DraftNext(ctx, bookID)
		a.setRunMessage("draft", accepted, err)
	case "audit":
		a.triggerChapterRun(ctx, "audit", fields[1:], s.books, a.client.AuditChapter)
	case "revise":
		a.triggerChapterRun(ctx, "revise", fields[1:], s.books, a.client.ReviseChapter)
	case "rewrite":
		a.triggerChapterRun(ctx, "rewrite", fields[1:], s.books, a.client.RewriteChapter)
	case "truth":
		bookID, ok := a.resolveBook(fields[1:], s.books)
		if !ok {
			return false
		}
		a.lastBookID = bookID
		if len(fields) >= 3 {
			a.renderTruthFile(ctx, bookID, fields[2])
		} else {
			a.renderTruthList(ctx, bookID)
		}
	case "truth-edit":
		bookID, ok := a.resolveBook(fields[1:], s.books)
		if !ok {
			return false
		}
		if len(fields) < 3 {
			a.message = "truth file is required"
			return false
		}
		a.lastBookID = bookID
		a.editTruthFile(ctx, bookID, fields[2])
	case "export":
		bookID, ok := a.resolveBook(fields[1:], s.books)
		if !ok {
			return false
		}
		format := "txt"
		if len(fields) >= 3 {
			format = fields[2]
		}
		result, err := a.client.ExportSave(ctx, bookID, format, false)
		if err != nil {
			a.message = "export failed: " + err.Error()
			return false
		}
		a.message = fmt.Sprintf("exported %d chapters to %s", result.Chapters, result.Path)
	case "logs":
		limit := 20
		if len(fields) >= 2 {
			if parsed, err := strconv.Atoi(fields[1]); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		logs, err := a.client.Logs(ctx, limit, "")
		if err != nil {
			a.message = "logs failed: " + err.Error()
			return false
		}
		a.renderJSON("Logs", logs)
	case "doctor":
		result, err := a.client.Doctor(ctx)
		if err != nil {
			a.message = "doctor failed: " + err.Error()
			return false
		}
		a.renderJSON("Doctor", result)
	case "config":
		result, err := a.client.Project(ctx)
		if err != nil {
			a.message = "config failed: " + err.Error()
			return false
		}
		a.renderJSON("Project Config", result)
	case "models":
		result, err := a.client.ModelRoutes(ctx)
		if err != nil {
			a.message = "models failed: " + err.Error()
			return false
		}
		a.renderJSON("Model Routes", result)
	case "profile-test":
		if len(fields) < 2 {
			a.message = "profile name is required"
			return false
		}
		result, err := a.client.TestProfile(ctx, fields[1])
		if err != nil {
			a.message = "profile test failed: " + err.Error()
			return false
		}
		a.renderJSON("Profile Test", result)
	case "genres":
		language := ""
		if len(fields) >= 2 {
			language = fields[1]
		}
		genres, err := a.client.ListGenres(ctx, language)
		if err != nil {
			a.message = "genres failed: " + err.Error()
			return false
		}
		a.renderGenres(genres)
	default:
		a.message = "unknown command: " + fields[0]
	}
	return false
}

func (a *App) resolveBook(args []string, books []BookSummary) (string, bool) {
	if len(args) == 0 {
		if a.lastBookID != "" {
			return a.lastBookID, true
		}
		a.message = "book id or list number is required"
		return "", false
	}
	raw := args[0]
	if idx, err := strconv.Atoi(raw); err == nil {
		if idx <= 0 || idx > len(books) {
			a.message = "book number out of range"
			return "", false
		}
		return books[idx-1].ID, true
	}
	return raw, true
}

func (a *App) resolveBookChapter(args []string, books []BookSummary) (string, int, bool) {
	if len(args) == 0 {
		a.message = "book and chapter number are required"
		return "", 0, false
	}
	if len(args) == 1 && a.lastBookID != "" {
		chapter, err := strconv.Atoi(args[0])
		if err != nil || chapter <= 0 {
			a.message = "chapter number must be positive"
			return "", 0, false
		}
		return a.lastBookID, chapter, true
	}
	bookID, ok := a.resolveBook(args[:1], books)
	if !ok {
		return "", 0, false
	}
	if len(args) < 2 {
		a.message = "chapter number is required"
		return "", 0, false
	}
	chapter, err := strconv.Atoi(args[1])
	if err != nil || chapter <= 0 {
		a.message = "chapter number must be positive"
		return "", 0, false
	}
	return bookID, chapter, true
}

func (a *App) createBook(ctx context.Context) {
	input := BookCreateInput{
		ID:               a.promptDefault("book id", ""),
		Title:            a.promptDefault("title", ""),
		Genre:            a.promptDefault("genre", "short"),
		Brief:            a.promptDefault("brief", ""),
		Language:         a.promptDefault("language", "zh"),
		Platform:         a.promptDefault("platform", "original"),
		TargetChapters:   a.promptIntDefault("target chapters", 10),
		ChapterWordCount: a.promptIntDefault("chapter word count", 8000),
	}
	book, err := a.client.CreateBook(ctx, input)
	if err != nil {
		a.message = "create book failed: " + err.Error()
		return
	}
	a.lastBookID = book.ID
	a.message = "created book: " + book.ID
}

func (a *App) handleDaemon(ctx context.Context, args []string, s snapshot) {
	action := "toggle"
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "start":
		status, err := a.client.SetDaemon(ctx, true)
		a.setDaemonMessage(status, err)
	case "stop":
		status, err := a.client.SetDaemon(ctx, false)
		a.setDaemonMessage(status, err)
	case "poll":
		status, err := a.client.PollDaemon(ctx)
		a.setDaemonMessage(status, err)
	case "toggle":
		if s.daemon == nil {
			a.message = "daemon status unavailable"
			return
		}
		status, err := a.client.SetDaemon(ctx, !s.daemon.Running)
		a.setDaemonMessage(status, err)
	default:
		a.message = "unknown daemon action: " + action
	}
}

func (a *App) setDaemonMessage(status *DaemonStatus, err error) {
	if err != nil {
		a.message = "daemon update failed: " + err.Error()
		return
	}
	a.message = "daemon " + runningText(status.Running)
}

func (a *App) triggerChapterRun(ctx context.Context, action string, args []string, books []BookSummary, fn func(context.Context, string, int) (*RunAccepted, error)) {
	bookID, chapter, ok := a.resolveBookChapter(args, books)
	if !ok {
		return
	}
	a.lastBookID = bookID
	accepted, err := fn(ctx, bookID, chapter)
	a.setRunMessage(action, accepted, err)
}

func (a *App) setRunMessage(action string, accepted *RunAccepted, err error) {
	if err != nil {
		a.message = action + " failed: " + err.Error()
		return
	}
	a.message = action + " accepted"
	if accepted != nil && accepted.RunID != "" {
		a.message += ": " + accepted.RunID
	}
}

func (a *App) renderHelp() {
	if !a.noClear {
		fmt.Print("\033[2J\033[H")
	}
	fmt.Println("StoryForge TUI Commands")
	fmt.Println(strings.Repeat("=", 64))
	fmt.Println("new                                create a book through the Web API")
	fmt.Println("book <book>                        show book config")
	fmt.Println("chapters <book>                    list chapters")
	fmt.Println("read <book> <n>                    show chapter content")
	fmt.Println("edit <book> <n>                    edit chapter content with $EDITOR")
	fmt.Println("approve|reject|analyze <book> <n>  review chapter")
	fmt.Println("write|draft <book>                 trigger next chapter run")
	fmt.Println("audit|revise|rewrite <book> <n>    trigger chapter agent run")
	fmt.Println("truth <book> [file]                list or show truth files")
	fmt.Println("truth-edit <book> <file>           edit truth JSON with $EDITOR")
	fmt.Println("export <book> [txt|md|epub]        save export artifact")
	fmt.Println("daemon start|stop|poll|toggle      control daemon mode")
	fmt.Println("logs [limit] | doctor | config | models | profile-test <name> | genres [language]")
	fmt.Println()
	fmt.Println("Press Enter to return.")
	_, _ = a.in.ReadString('\n')
}

func (a *App) renderChapters(ctx context.Context, bookID string) {
	chapters, err := a.client.ListChapters(ctx, bookID)
	if err != nil {
		a.message = "list chapters failed: " + err.Error()
		return
	}
	if !a.noClear {
		fmt.Print("\033[2J\033[H")
	}
	fmt.Printf("Chapters: %s\n", bookID)
	fmt.Println(strings.Repeat("=", 64))
	if len(chapters) == 0 {
		fmt.Println("No chapters yet.")
	} else {
		for _, chapter := range chapters {
			fmt.Printf("%4d  %-14s %6d words  %s\n",
				chapter.Number,
				chapter.Status,
				chapter.WordCount,
				trim(chapter.Title, 36),
			)
		}
	}
	fmt.Println()
	fmt.Println("Press Enter to return.")
	_, _ = a.in.ReadString('\n')
}

func (a *App) renderBook(ctx context.Context, bookID string) {
	book, err := a.client.GetBook(ctx, bookID)
	if err != nil {
		a.message = "get book failed: " + err.Error()
		return
	}
	a.renderJSON("Book: "+bookID, book)
}

func (a *App) renderChapter(ctx context.Context, bookID string, chapter int) {
	detail, err := a.client.GetChapter(ctx, bookID, chapter)
	if err != nil {
		a.message = "get chapter failed: " + err.Error()
		return
	}
	if !a.noClear {
		fmt.Print("\033[2J\033[H")
	}
	fmt.Printf("Chapter %s #%d\n", bookID, chapter)
	fmt.Println(strings.Repeat("=", 64))
	fmt.Printf("%s  %s  %d words\n\n", detail.Meta.Title, detail.Meta.Status, detail.Meta.WordCount)
	fmt.Println(detail.Content)
	fmt.Println()
	fmt.Println("Press Enter to return.")
	_, _ = a.in.ReadString('\n')
}

func (a *App) editChapter(ctx context.Context, bookID string, chapter int) {
	detail, err := a.client.GetChapter(ctx, bookID, chapter)
	if err != nil {
		a.message = "get chapter failed: " + err.Error()
		return
	}
	edited, err := a.editText(detail.Content, "*.md")
	if err != nil {
		a.message = "edit failed: " + err.Error()
		return
	}
	if edited == detail.Content {
		a.message = "chapter unchanged"
		return
	}
	if err := a.client.SaveChapter(ctx, bookID, chapter, edited); err != nil {
		a.message = "save chapter failed: " + err.Error()
		return
	}
	a.message = fmt.Sprintf("chapter saved: %s #%d", bookID, chapter)
}

func (a *App) renderTruthList(ctx context.Context, bookID string) {
	files, err := a.client.ListTruth(ctx, bookID)
	if err != nil {
		a.message = "truth list failed: " + err.Error()
		return
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	if !a.noClear {
		fmt.Print("\033[2J\033[H")
	}
	fmt.Printf("Truth Files: %s\n", bookID)
	fmt.Println(strings.Repeat("=", 64))
	for _, name := range names {
		fmt.Println(name)
	}
	fmt.Println()
	fmt.Println("Press Enter to return.")
	_, _ = a.in.ReadString('\n')
}

func (a *App) renderTruthFile(ctx context.Context, bookID, file string) {
	data, err := a.client.GetTruthFile(ctx, bookID, file)
	if err != nil {
		a.message = "truth file failed: " + err.Error()
		return
	}
	a.renderRawJSON("Truth: "+bookID+"/"+file, data)
}

func (a *App) editTruthFile(ctx context.Context, bookID, file string) {
	data, err := a.client.GetTruthFile(ctx, bookID, file)
	if err != nil {
		a.message = "truth file failed: " + err.Error()
		return
	}
	initial := string(data)
	if formatted, err := json.MarshalIndent(json.RawMessage(data), "", "  "); err == nil {
		initial = string(formatted) + "\n"
	}
	edited, err := a.editText(initial, "*.json")
	if err != nil {
		a.message = "edit failed: " + err.Error()
		return
	}
	edited = strings.TrimSpace(edited)
	if !json.Valid([]byte(edited)) {
		a.message = "truth update cancelled: edited content is not valid JSON"
		return
	}
	if err := a.client.UpdateTruthFile(ctx, bookID, file, json.RawMessage(edited)); err != nil {
		a.message = "truth save failed: " + err.Error()
		return
	}
	a.message = "truth file saved: " + file
}

func (a *App) renderGenres(genres []GenreSummary) {
	if !a.noClear {
		fmt.Print("\033[2J\033[H")
	}
	fmt.Println("Genres")
	fmt.Println(strings.Repeat("=", 64))
	for _, genre := range genres {
		fmt.Printf("%-8s %-24s %s\n", genre.Language, genre.ID, genre.Name)
	}
	fmt.Println()
	fmt.Println("Press Enter to return.")
	_, _ = a.in.ReadString('\n')
}

func (a *App) renderJSON(title string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		a.message = "render JSON failed: " + err.Error()
		return
	}
	a.renderRawJSON(title, data)
}

func (a *App) renderRawJSON(title string, data []byte) {
	if !a.noClear {
		fmt.Print("\033[2J\033[H")
	}
	fmt.Println(title)
	fmt.Println(strings.Repeat("=", 64))
	fmt.Println(string(data))
	fmt.Println()
	fmt.Println("Press Enter to return.")
	_, _ = a.in.ReadString('\n')
}

func (a *App) promptDefault(label, fallback string) string {
	if fallback == "" {
		fmt.Printf("%s: ", label)
	} else {
		fmt.Printf("%s [%s]: ", label, fallback)
	}
	line, _ := a.in.ReadString('\n')
	value := strings.TrimSpace(line)
	if value == "" {
		return fallback
	}
	return value
}

func (a *App) promptIntDefault(label string, fallback int) int {
	raw := a.promptDefault(label, fmt.Sprintf("%d", fallback))
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func (a *App) confirm(prompt string) bool {
	fmt.Print(prompt)
	line, _ := a.in.ReadString('\n')
	return strings.TrimSpace(strings.ToLower(line)) == "yes"
}

func (a *App) editText(initial, pattern string) (string, error) {
	file, err := os.CreateTemp("", "storyforge-tui-"+pattern)
	if err != nil {
		return "", err
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := file.WriteString(initial); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func okText(ok bool) string {
	if ok {
		return "ok"
	}
	return "down"
}

func runningText(running bool) string {
	if running {
		return "running"
	}
	return "stopped"
}

func trim(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}

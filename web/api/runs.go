package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"storyforge/internal/app"
	"storyforge/internal/model"
	"storyforge/internal/run"
)

type pipelineHandler struct {
	svc         *app.PipelineService
	review      *app.ReviewService
	chapters    *app.ChaptersService
	broadcaster *run.Broadcaster
	events      *EventBus
}

func (h *pipelineHandler) triggerWrite(w http.ResponseWriter, r *http.Request) {
	h.trigger(w, r, model.RunKindFullPipeline)
}

func (h *pipelineHandler) triggerPlan(w http.ResponseWriter, r *http.Request) {
	h.trigger(w, r, model.RunKindPlan)
}

func (h *pipelineHandler) triggerCompose(w http.ResponseWriter, r *http.Request) {
	h.trigger(w, r, model.RunKindCompose)
}

func (h *pipelineHandler) triggerDraft(w http.ResponseWriter, r *http.Request) {
	h.trigger(w, r, model.RunKindWrite)
}

func (h *pipelineHandler) triggerWriteNext(w http.ResponseWriter, r *http.Request) {
	h.triggerNext(w, r, model.RunKindFullPipeline)
}

func (h *pipelineHandler) triggerDraftNext(w http.ResponseWriter, r *http.Request) {
	h.triggerNext(w, r, model.RunKindWrite)
}

func (h *pipelineHandler) triggerAudit(w http.ResponseWriter, r *http.Request) {
	h.triggerChapterPath(w, r, model.RunKindAudit)
}

func (h *pipelineHandler) triggerRevise(w http.ResponseWriter, r *http.Request) {
	h.triggerChapterPath(w, r, model.RunKindRevise)
}

func (h *pipelineHandler) rewrite(w http.ResponseWriter, r *http.Request) {
	bookID := pathParam(r, "bookID")
	chapter, err := parseChapterNum(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.review == nil {
		writeError(w, http.StatusInternalServerError, "review service unavailable")
		return
	}
	if h.events != nil {
		h.events.Publish("rewrite:start", map[string]any{"bookId": bookID, "chapter": chapter})
	}

	rejectResult, err := h.review.Reject(r.Context(), bookID, chapter, "rewrite")
	if err != nil {
		if h.events != nil {
			h.events.Publish("rewrite:error", map[string]any{"bookId": bookID, "chapter": chapter, "error": err.Error()})
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	runRecord, err := h.svc.Trigger(r.Context(), app.TriggerInput{
		BookID:      bookID,
		Chapter:     chapter,
		Kind:        model.RunKindFullPipeline,
		TriggeredBy: model.RunTriggeredByStudio,
	})
	if err != nil {
		if h.events != nil {
			h.events.Publish("rewrite:error", map[string]any{"bookId": bookID, "chapter": chapter, "error": err.Error()})
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.bridgeRunLifecycle(runRecord.ID, "rewrite", bookID, chapter)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"runId":             runRecord.ID,
		"rollbackToChapter": rejectResult.RollbackToChapter,
		"deletedFrom":       rejectResult.DeletedFrom,
	})
}

func (h *pipelineHandler) trigger(w http.ResponseWriter, r *http.Request, kind model.RunKind) {
	bookID := pathParam(r, "bookID")
	var body struct {
		Chapter int `json:"chapter"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Chapter <= 0 && (kind == model.RunKindFullPipeline || kind == model.RunKindWrite) {
		nextChapter, err := h.nextChapter(bookID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		body.Chapter = nextChapter
	}
	run, err := h.svc.Trigger(r.Context(), app.TriggerInput{
		BookID:      bookID,
		Chapter:     body.Chapter,
		Kind:        kind,
		TriggeredBy: model.RunTriggeredByStudio,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.publishRunStart(kind, bookID, body.Chapter)
	h.bridgeRunLifecycle(run.ID, runEventPrefix(kind), bookID, body.Chapter)
	writeJSON(w, http.StatusAccepted, map[string]string{"runId": run.ID})
}

func (h *pipelineHandler) triggerChapterPath(w http.ResponseWriter, r *http.Request, kind model.RunKind) {
	bookID := pathParam(r, "bookID")
	chapter, err := parseChapterNum(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body struct {
		Mode string `json:"mode"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
	}
	mode := ""
	if kind == model.RunKindRevise {
		mode = app.NormalizeReviseMode(body.Mode)
	}
	runRecord, err := h.svc.Trigger(r.Context(), app.TriggerInput{
		BookID:      bookID,
		Chapter:     chapter,
		Kind:        kind,
		ReviseMode:  mode,
		TriggeredBy: model.RunTriggeredByStudio,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.publishRunStart(kind, bookID, chapter)
	h.bridgeRunLifecycle(runRecord.ID, runEventPrefix(kind), bookID, chapter)
	resp := map[string]any{"runId": runRecord.ID}
	if mode != "" {
		resp["mode"] = mode
	}
	writeJSON(w, http.StatusAccepted, resp)
}

func (h *pipelineHandler) publishRunStart(kind model.RunKind, bookID string, chapter int) {
	if h.events == nil {
		return
	}
	prefix := runEventPrefix(kind)
	if prefix == "" {
		return
	}
	h.events.Publish(prefix+":start", map[string]any{"bookId": bookID, "chapter": chapter})
}

func (h *pipelineHandler) bridgeRunLifecycle(runID, prefix, bookID string, chapter int) {
	if h.events == nil || h.broadcaster == nil || prefix == "" {
		return
	}
	ch, cancel := h.broadcaster.Subscribe(runID)
	go func() {
		defer cancel()
		for event := range ch {
			switch event.Type {
			case "complete":
				h.events.Publish(prefix+":complete", map[string]any{"bookId": bookID, "chapter": chapter, "runId": runID})
				return
			case "error":
				h.events.Publish(prefix+":error", map[string]any{"bookId": bookID, "chapter": chapter, "runId": runID, "error": event.Message})
				return
			}
		}
	}()
}

func runEventPrefix(kind model.RunKind) string {
	switch kind {
	case model.RunKindFullPipeline:
		return "write"
	case model.RunKindWrite:
		return "draft"
	case model.RunKindAudit:
		return "audit"
	case model.RunKindRevise:
		return "revise"
	default:
		return ""
	}
}

func (h *pipelineHandler) listRuns(w http.ResponseWriter, r *http.Request) {
	bookID := pathParam(r, "bookID")
	runs, err := h.svc.ListRuns(bookID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if runs == nil {
		runs = []*model.Run{}
	}
	writeJSON(w, http.StatusOK, runs)
}

func (h *pipelineHandler) triggerNext(w http.ResponseWriter, r *http.Request, kind model.RunKind) {
	bookID := pathParam(r, "bookID")
	nextChapter, err := h.nextChapter(bookID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	runRecord, err := h.svc.Trigger(r.Context(), app.TriggerInput{
		BookID:      bookID,
		Chapter:     nextChapter,
		Kind:        kind,
		TriggeredBy: model.RunTriggeredByStudio,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.publishRunStart(kind, bookID, nextChapter)
	h.bridgeRunLifecycle(runRecord.ID, runEventPrefix(kind), bookID, nextChapter)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"runId":   runRecord.ID,
		"chapter": nextChapter,
	})
}

func (h *pipelineHandler) nextChapter(bookID string) (int, error) {
	if h.chapters == nil {
		return 0, fmt.Errorf("chapters service unavailable")
	}
	metas, err := h.chapters.List(bookID)
	if err != nil {
		return 0, err
	}
	nextChapter := 1
	for _, meta := range metas {
		if meta.Number >= nextChapter {
			nextChapter = meta.Number + 1
		}
	}
	return nextChapter, nil
}

// runsHandler handles global run endpoints.
type runsHandler struct {
	svc         *app.PipelineService
	broadcaster *run.Broadcaster
}

func (h *runsHandler) get(w http.ResponseWriter, r *http.Request) {
	runID := pathParam(r, "runID")
	// We need bookID — for now accept it as a query param
	bookID := r.URL.Query().Get("bookId")
	if bookID == "" {
		writeError(w, http.StatusBadRequest, "bookId query param required")
		return
	}
	runRecord, err := h.svc.GetRun(bookID, runID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, runRecord)
}

func (h *runsHandler) traces(w http.ResponseWriter, r *http.Request) {
	runID := pathParam(r, "runID")
	traces, err := h.svc.GetTraces(runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if traces == nil {
		traces = []*model.PromptTrace{}
	}
	writeJSON(w, http.StatusOK, traces)
}

// events streams run events as Server-Sent Events.
func (h *runsHandler) events(w http.ResponseWriter, r *http.Request) {
	runID := pathParam(r, "runID")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	ch, cancel := h.broadcaster.Subscribe(runID)
	defer cancel()

	for {
		select {
		case event, open := <-ch:
			if !open {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			if event.Type == "complete" || event.Type == "error" {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

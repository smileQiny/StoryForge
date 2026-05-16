package api

import (
	"encoding/json"
	"net/http"

	"storyforge/internal/app"
	"storyforge/internal/model"
)

type booksHandler struct {
	svc *app.BooksService
}

func (h *booksHandler) list(w http.ResponseWriter, r *http.Request) {
	books, err := h.svc.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if books == nil {
		books = []*model.BookConfig{}
	}
	writeJSON(w, http.StatusOK, books)
}

func (h *booksHandler) create(w http.ResponseWriter, r *http.Request) {
	var input app.CreateBookInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	book, err := h.svc.Create(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, book)
}

func (h *booksHandler) get(w http.ResponseWriter, r *http.Request) {
	bookID := pathParam(r, "bookID")
	book, err := h.svc.Get(bookID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, book)
}

func (h *booksHandler) update(w http.ResponseWriter, r *http.Request) {
	bookID := pathParam(r, "bookID")
	var input app.UpdateBookInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	book, err := h.svc.Update(bookID, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, book)
}

func (h *booksHandler) delete(w http.ResponseWriter, r *http.Request) {
	bookID := pathParam(r, "bookID")
	if err := h.svc.Delete(bookID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

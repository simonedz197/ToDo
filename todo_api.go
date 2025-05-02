package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"log/slog"
	"net/http"
	"os"
	list "tut2/todo/todolist"
)

var logFile, err = os.OpenFile("todo.log", os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
var baseHandler = slog.NewTextHandler(logFile, &slog.HandlerOptions{AddSource: true})
var customHandler = &ContextHandler{Handler: baseHandler}
var logger = slog.New(customHandler)

func init() {
	slog.SetDefault(logger)
}

type ContextHandler struct {
	slog.Handler
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if requestID, ok := ctx.Value("request_id").(string); ok {
		r.AddAttrs(slog.String("request_id", requestID))
	}
	if userID, ok := ctx.Value("user_id").(string); ok {
		r.AddAttrs(slog.String("user_id", userID))
	}
	return h.Handler.Handle(ctx, r)
}

// create a dummy context
func dummyContext() context.Context {
	request_id := uuid.NewString()
	user_id := "edz197"

	ctx := context.Background()
	ctx = context.WithValue(ctx, "request_id", request_id)
	ctx = context.WithValue(ctx, "user_id", user_id)
	return ctx
}

type todoHandler struct{}

type putBody struct {
	item        string
	replacewith string
}

type deleteBody struct {
	item int
	task string
}

func (h *todoHandler) Get(w http.ResponseWriter, r *http.Request) {
	todolist := list.SortedMap()
	j, err := json.Marshal(todolist)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(j)
}

func (h *todoHandler) Post(w http.ResponseWriter, r *http.Request) {
	var pb = make(map[string]string)
	err := json.NewDecoder(r.Body).Decode(&pb)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	err = list.AddEntry(pb["item"])
	if err != nil {
		if errors.Is(err, list.AlreadyExistsErr) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Already Exists"))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	} else {
		h.Get(w, r)
	}
}

func (h *todoHandler) Put(w http.ResponseWriter, r *http.Request) {
	var pb = make(map[string]string)
	err := json.NewDecoder(r.Body).Decode(&pb)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	// make sure we have an item and a replacewith
	item := pb["item"]
	replaceWith := pb["replacewith"]
	w.Header().Set("Content-Type", "application/json")
	if item == "" || replaceWith == "" {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		err := list.UpdateEntry(item, replaceWith)
		if err != nil {
			if errors.Is(err, list.NotFoundErr) {
				w.WriteHeader(http.StatusNotFound)
			} else {
				w.WriteHeader(http.StatusInternalServerError)
			}
		} else {
			h.Get(w, r)
		}
	}
}

func (h *todoHandler) Delete(w http.ResponseWriter, r *http.Request) {
	var db = make(map[string]string)
	err := json.NewDecoder(r.Body).Decode(&db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	err = list.DeleteEntry(db["item"])
	if err != nil {
		if errors.Is(err, list.NotFoundErr) {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	} else {
		h.Get(w, r)
	}

}

func main() {
	ctx := dummyContext()
	if err := list.LoadEntries(); err != nil {
		logger.ErrorContext(ctx, "Error Loading todo List", "details", err)
		panic(errors.New(fmt.Sprintf("Error Loading todo List %v", err)))
	}

	mux := http.NewServeMux()
	h := todoHandler{}

	mux.HandleFunc("GET /todo", h.Get)
	mux.HandleFunc("POST /todo", h.Post)
	mux.HandleFunc("PUT /todo", h.Put)
	mux.HandleFunc("DELETE /todo", h.Delete)
	fmt.Printf("\nListening on port 8000\n")
	if err := http.ListenAndServe(":8000", mux); err != nil {
		fmt.Printf("error running http server: %s\n", err)
	}
}

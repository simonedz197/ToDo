package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	list "tut2/todo/todolist"
)

var logFile, err = os.OpenFile("todo.log", os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
var baseHandler = slog.NewTextHandler(logFile, &slog.HandlerOptions{AddSource: true})
var customHandler = &ContextHandler{Handler: baseHandler}
var logger = slog.New(customHandler)

type RequestData struct {
	Writer  http.ResponseWriter
	Request *http.Request
}

var Queue = make(chan RequestData)

const (
	xRequestId = "X-Request-ID"
)

func TracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestId := r.Header.Get(xRequestId)
		if requestId == "" {
			requestId = uuid.NewString()
		}
		ctx := context.WithValue(r.Context(), xRequestId, requestId)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func init() {
	slog.SetDefault(logger)
}

type ContextHandler struct {
	slog.Handler
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if traceid, ok := ctx.Value(xRequestId).(string); ok {
		r.AddAttrs(slog.String("trace_id", traceid))
	}
	if userID, ok := ctx.Value("user_id").(string); ok {
		r.AddAttrs(slog.String("user_id", userID))
	}
	return h.Handler.Handle(ctx, r)
}

type todoPageData struct {
	PageTitle string
	Items     []list.ToDoItem
}

var postTodo = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	logger.InfoContext(r.Context(), "post request received")
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
		serveTemplate(w, r)
	}
	return
})

var putTodo = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	logger.InfoContext(r.Context(), "put request received")
	var pb = make(map[string]string)
	err := json.NewDecoder(r.Body).Decode(&pb)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	item := pb["item"]
	replaceWith := pb["replacewith"]
	w.Header().Set("Content-Type", "application/json")
	if item == "" || replaceWith == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err = list.UpdateEntry(item, replaceWith)
	if err != nil {
		if errors.Is(err, list.NotFoundErr) {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	} else {
		serveTemplate(w, r)
	}
	return
})

var deleteTodo = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	logger.InfoContext(r.Context(), "delete request received")
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
		serveTemplate(w, r)
	}
	return
})

var serveTemplate = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	lp := filepath.Join("dynamic", "layout.html")
	data := todoPageData{
		PageTitle: "TO DO LIST",
	}
	data.Items = list.SortedMap()

	tmpl, err := template.New("layout.html").ParseFiles(lp)
	if err != nil {
		logger.ErrorContext(r.Context(), "error parsing list template", err)
		return
	}
	err = tmpl.Execute(w, data)
	if err != nil {
		logger.ErrorContext(r.Context(), "error executing list template", err)
	}
	return
})

func ProcessQueue() {
	for v := range Queue {
		// get method and log request
		requestlog := fmt.Sprintf("Process %s Request", v.Request.Method)
		logger.InfoContext(v.Request.Context(), requestlog)

	}
}

func main() {
	ctx := context.Background()
	if err := list.LoadEntries(); err != nil {
		logger.ErrorContext(ctx, "Error Loading todo List", "details", err)
		panic(errors.New(fmt.Sprintf("Error Loading todo List %v", err)))
	}

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./static"))

	mux.Handle("GET /todo", TracingMiddleware(serveTemplate))
	mux.Handle("POST /todo", TracingMiddleware(postTodo))
	mux.Handle("PUT /todo", TracingMiddleware(putTodo))
	mux.Handle("DELETE /todo", TracingMiddleware(deleteTodo))
	mux.Handle("/todo/", http.StripPrefix("/todo/", fs))
	fmt.Printf("\nListening on port 8000\n")
	if err := http.ListenAndServe(":8000", mux); err != nil {
		fmt.Printf("error running http server: %s\n", err)
	}
}

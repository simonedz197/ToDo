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
	"strings"
	list "tut2/todo/todolist"
)

var logFile, err = os.OpenFile("todo.log", os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
var baseHandler = slog.NewTextHandler(logFile, &slog.HandlerOptions{AddSource: true})
var customHandler = &ContextHandler{Handler: baseHandler}
var logger = slog.New(customHandler)

type RequestJob struct {
	Writer  http.ResponseWriter
	Request *http.Request
	done    chan bool
}

var Queue = make(chan RequestJob)

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

func postRequest(job RequestJob) {
	var pb = make(map[string]string)
	err := json.NewDecoder(job.Request.Body).Decode(&pb)
	if err != nil {
		logger.ErrorContext(job.Request.Context(), fmt.Sprintf("%v", err))
		http.Error(job.Writer, err.Error(), http.StatusBadRequest)
		job.done <- true
		return
	}
	err = list.AddEntry(pb["item"])
	if err != nil {
		logger.ErrorContext(job.Request.Context(), fmt.Sprintf("%v", err))
		if errors.Is(err, list.AlreadyExistsErr) {
			job.Writer.Write([]byte("Already Exists"))
		} else {
			job.Writer.WriteHeader(http.StatusInternalServerError)
		}
	}
	job.done <- true
	return
}

func putRequest(job RequestJob) {
	var pb = make(map[string]string)
	err := json.NewDecoder(job.Request.Body).Decode(&pb)
	if err != nil {
		logger.ErrorContext(job.Request.Context(), fmt.Sprintf("%v", err))
		http.Error(job.Writer, err.Error(), http.StatusBadRequest)
		job.done <- true
	}
	item := pb["item"]
	replaceWith := pb["replacewith"]
	job.Writer.Header().Set("Content-Type", "application/json")
	if item == "" || replaceWith == "" {
		job.Writer.WriteHeader(http.StatusBadRequest)
		job.done <- true
		return
	}
	err = list.UpdateEntry(item, replaceWith)
	if err != nil {
		logger.ErrorContext(job.Request.Context(), fmt.Sprintf("%v", err))
		if errors.Is(err, list.NotFoundErr) {
			job.Writer.WriteHeader(http.StatusNotFound)
		} else {
			job.Writer.WriteHeader(http.StatusInternalServerError)
		}
	}
	job.done <- true
	return
}

func deleteRequest(job RequestJob) {
	var db = make(map[string]string)
	err := json.NewDecoder(job.Request.Body).Decode(&db)
	if err != nil {
		logger.ErrorContext(job.Request.Context(), fmt.Sprintf("%v", err))
		job.Writer.WriteHeader(http.StatusBadRequest)
		job.done <- true
		return
	}
	err = list.DeleteEntry(db["item"])
	if err != nil {
		logger.ErrorContext(job.Request.Context(), fmt.Sprintf("%v", err))
		if errors.Is(err, list.NotFoundErr) {
			job.Writer.WriteHeader(http.StatusNotFound)
		} else {
			job.Writer.WriteHeader(http.StatusInternalServerError)
		}
	}
	job.done <- true
	return
}

func serveTemplate(job RequestJob) {
	logger.InfoContext(job.Request.Context(), "Serving Template")
	lp := filepath.Join("dynamic", "layout.html")
	data := todoPageData{
		PageTitle: "TO DO LIST",
	}
	data.Items = list.SortedMap()

	tmpl, err := template.New("layout.html").ParseFiles(lp)
	if err != nil {
		logger.ErrorContext(job.Request.Context(), "error parsing list template", err)
		job.done <- true
		return
	}
	err = tmpl.Execute(job.Writer, data)
	if err != nil {
		logger.ErrorContext(job.Request.Context(), "error executing list template", err)
	}
	job.done <- true
	return
}

func ProcessQueue() {
	for v := range Queue {
		// get method and log request
		requestlog := fmt.Sprintf("Process %s Request", v.Request.Method)
		logger.InfoContext(v.Request.Context(), requestlog)
		switch strings.ToUpper(v.Request.Method) {
		case "POST":
			postRequest(v)
		case "PUT":
			putRequest(v)
		case "DELETE":
			deleteRequest(v)
		case "GET":
			serveTemplate(v)
		default:
			v.Writer.WriteHeader(http.StatusMethodNotAllowed)
			v.done <- true
		}
	}
}

var ProcessRequest = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	data := RequestJob{w, r, make(chan bool)}
	Queue <- data
	<-data.done
})

func main() {
	ctx := context.Background()
	if err := list.LoadEntries(); err != nil {
		logger.ErrorContext(ctx, "Error Loading todo List", "details", err)
		panic(errors.New(fmt.Sprintf("Error Loading todo List %v", err)))
	}

	go ProcessQueue()

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./static"))

	mux.Handle("/todo", TracingMiddleware(ProcessRequest))
	mux.Handle("/todo/", http.StripPrefix("/todo/", fs))
	fmt.Printf("\nListening on port 8000\n")
	if err := http.ListenAndServe(":8000", mux); err != nil {
		fmt.Printf("error running http server: %s\n", err)
	}
}

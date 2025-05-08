package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"html/template"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	list "tut2/todo/todolist"
)

type RequestJob struct {
	Writer  http.ResponseWriter
	Request *http.Request
	done    chan struct{}
}

var Queue = make(chan RequestJob)

type todoPageData struct {
	PageTitle string
	Items     []list.ToDoItem
}

func TracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestId := r.Header.Get("X-Request-ID")
		if requestId == "" {
			requestId = uuid.NewString()
		}
		ctx := context.WithValue(r.Context(), "X-Request-ID", requestId)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func postRequest(job RequestJob) {
	defer close(job.done)
	var pb = make(map[string]string)
	err := json.NewDecoder(job.Request.Body).Decode(&pb)
	if err != nil {
		list.Logger.ErrorContext(job.Request.Context(), fmt.Sprintf("%v", err))
		http.Error(job.Writer, err.Error(), http.StatusBadRequest)
		return
	}
	data := list.DataStoreJob{job.Request.Context(), list.AddData, pb["item"], "", make(chan list.ReturnChannelData)}
	list.DataJobQueue <- data
	returnVal, ok := <-data.ReturnChannel
	if ok {
		if returnVal.Err != nil {
			list.Logger.ErrorContext(job.Request.Context(), fmt.Sprintf("%v", returnVal.Err))
			if errors.Is(returnVal.Err, list.AlreadyExistsErr) {
				job.Writer.Write([]byte("Already Exists"))
			} else {
				job.Writer.WriteHeader(http.StatusInternalServerError)
			}
		}
	}
	return
}

func putRequest(job RequestJob) {
	defer close(job.done)
	var pb = make(map[string]string)
	err := json.NewDecoder(job.Request.Body).Decode(&pb)
	if err != nil {
		list.Logger.ErrorContext(job.Request.Context(), fmt.Sprintf("%v", err))
		http.Error(job.Writer, err.Error(), http.StatusBadRequest)
		return
	}

	if pb["item"] == "" || pb["replacewith"] == "" {
		job.Writer.WriteHeader(http.StatusBadRequest)
		return
	}
	data := list.DataStoreJob{job.Request.Context(), list.UpdateData, pb["item"], pb["replacewith"], make(chan list.ReturnChannelData)}
	list.DataJobQueue <- data
	returnVal, ok := <-data.ReturnChannel
	if ok {
		if returnVal.Err != nil {
			list.Logger.ErrorContext(job.Request.Context(), fmt.Sprintf("%v", returnVal.Err))
			if errors.Is(returnVal.Err, list.NotFoundErr) {
				job.Writer.WriteHeader(http.StatusNotFound)
			} else {
				job.Writer.WriteHeader(http.StatusInternalServerError)
			}
		}
	}
	return
}

func deleteRequest(job RequestJob) {
	defer close(job.done)
	var db = make(map[string]string)
	err := json.NewDecoder(job.Request.Body).Decode(&db)
	if err != nil {
		list.Logger.ErrorContext(job.Request.Context(), fmt.Sprintf("%v", err))
		job.Writer.WriteHeader(http.StatusBadRequest)
		return
	}
	data := list.DataStoreJob{job.Request.Context(), list.UpdateData, db["item"], "", make(chan list.ReturnChannelData)}
	list.DataJobQueue <- data
	returnVal, ok := <-data.ReturnChannel
	if ok {
		if returnVal.Err != nil {
			list.Logger.ErrorContext(job.Request.Context(), fmt.Sprintf("%v", returnVal.Err))
			if errors.Is(returnVal.Err, list.NotFoundErr) {
				job.Writer.WriteHeader(http.StatusNotFound)
			} else {
				job.Writer.WriteHeader(http.StatusInternalServerError)
			}
		}
	}
	return
}

func serveTemplate(job RequestJob) {
	defer close(job.done)
	list.Logger.InfoContext(job.Request.Context(), "Serving Template")
	lp := filepath.Join("dynamic", "layout.html")
	pageData := todoPageData{
		PageTitle: "TO DO LIST",
	}

	data := list.DataStoreJob{job.Request.Context(), list.FetchData, "", "", make(chan list.ReturnChannelData)}
	list.DataJobQueue <- data
	returnVal, ok := <-data.ReturnChannel
	if ok {
		if returnVal.Err != nil {
			list.Logger.ErrorContext(job.Request.Context(), fmt.Sprintf("%v", returnVal.Err))
			job.Writer.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	pageData.Items = list.SortedArray(returnVal.List)

	tmpl, err := template.New("layout.html").ParseFiles(lp)
	if err != nil {
		list.Logger.ErrorContext(job.Request.Context(), "error parsing list template")
		return
	}
	err = tmpl.Execute(job.Writer, pageData)
	if err != nil {
		list.Logger.ErrorContext(job.Request.Context(), "error executing list template")
	}
	return
}

func ProcessHttpQueue() {
	for v := range Queue {
		// get method and log request
		requestlog := fmt.Sprintf("Process %s Request", v.Request.Method)
		list.Logger.InfoContext(v.Request.Context(), requestlog)
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
		}
	}
}

var ProcessRequest = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	data := RequestJob{w, r, make(chan struct{})}
	Queue <- data
	<-data.done
})

func main() {
	ctx := context.Background()

	go list.ProcessDataJobs()
	go ProcessHttpQueue()

	data := list.DataStoreJob{ctx, list.LoadData, "", "", make(chan list.ReturnChannelData)}
	list.DataJobQueue <- data
	returnVal, ok := <-data.ReturnChannel
	if ok {
		if returnVal.Err != nil {
			list.Logger.ErrorContext(ctx, "Error Loading todo List", "details", returnVal.Err)
			return
		}
	}

	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Printf("\nclosing down...\n")
		data := list.DataStoreJob{ctx, list.StoreData, "", "", make(chan list.ReturnChannelData)}
		list.DataJobQueue <- data
		returnVal, ok := <-data.ReturnChannel
		if ok {
			if returnVal.Err != nil {
				list.Logger.ErrorContext(ctx, "Error saving todo List", "details", returnVal.Err)
				return
			}
		}
		os.Exit(1)
	}()

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./static"))

	mux.Handle("/todo", TracingMiddleware(ProcessRequest))
	mux.Handle("/todo/", http.StripPrefix("/todo/", fs))
	fmt.Printf("\nListening on port 8000\n")
	if err := http.ListenAndServe(":8000", mux); err != nil {
		fmt.Printf("error running http server: %s\n", err)
	}
}

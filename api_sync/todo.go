package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/google/uuid"
	list "github.com/simonedz197/ToDoListStore"
)

type RequestJob struct {
	Writer  http.ResponseWriter
	Request *http.Request
	uid     string
	done    chan struct{}
}

type RequetHeaderKey string

const IdRequestHeader = "X-Request-ID"

var Queue = make(chan RequestJob)

type todoPageData struct {
	PageTitle string
	Items     []list.ToDoItem
}

func TracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestId := r.Header.Get(IdRequestHeader)
		if requestId == "" {
			requestId = uuid.NewString()
		}
		ctx := context.WithValue(r.Context(), IdRequestHeader, requestId)
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
	data := list.DataStoreJob{Context: job.Request.Context(), Uid: job.uid, JobType: list.AddData, KeyValue: pb["item"], AltValue: "", ReturnChannel: make(chan list.ReturnChannelData)}
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
	data := list.DataStoreJob{Context: job.Request.Context(), Uid: job.uid, JobType: list.UpdateData, KeyValue: pb["item"], AltValue: pb["replacewith"], ReturnChannel: make(chan list.ReturnChannelData)}
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
	data := list.DataStoreJob{Context: job.Request.Context(), Uid: job.uid, JobType: list.UpdateData, KeyValue: db["item"], AltValue: "", ReturnChannel: make(chan list.ReturnChannelData)}
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
}

func serveTemplate(job RequestJob) {
	defer close(job.done)
	list.Logger.InfoContext(job.Request.Context(), "Serving Template")
	lp := filepath.Join("dynamic", "layout.html")

	pageData := todoPageData{
		PageTitle: "TO DO LIST FOR " + job.uid,
	}

	data := list.DataStoreJob{Context: job.Request.Context(), Uid: job.uid, JobType: list.FetchData, KeyValue: "", AltValue: "", ReturnChannel: make(chan list.ReturnChannelData)}
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
	//extract uid from url
	uid := "Anonymous User"
	err := r.ParseForm()
	if err != nil {
		uid = r.FormValue("uid")
	}
	data := RequestJob{w, r, uid, make(chan struct{})}
	Queue <- data
	<-data.done
})

var ProcessRequestWithoutActor = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	//extract uid from url
	uid := "Anonymous User"
	err := r.ParseForm()
	if err != nil {
		uid = r.FormValue("uid")
	}

	list.Logger.InfoContext(r.Context(), "processing http request")
	// create a stuct and call the appropriate function

	switch r.Method {
	case http.MethodPost:
		var pb = make(map[string]string)
		err := json.NewDecoder(r.Body).Decode(&pb)
		if err != nil {
			list.Logger.ErrorContext(r.Context(), fmt.Sprintf("%v", err))
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		err = list.BasicAddToDoItem(uid, pb["item"])
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	case http.MethodPut:
		var pb = make(map[string]string)
		err := json.NewDecoder(r.Body).Decode(&pb)
		if err != nil {
			list.Logger.ErrorContext(r.Context(), fmt.Sprintf("%v", err))
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		err = list.BasicUpdateToDoItem(uid, pb["item"], pb["replacewith"])
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	case http.MethodDelete:
		var pb = make(map[string]string)
		err := json.NewDecoder(r.Body).Decode(&pb)
		if err != nil {
			list.Logger.ErrorContext(r.Context(), fmt.Sprintf("%v", err))
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		err = list.BasicDeleteToDoItem(uid, pb["item"])
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	case http.MethodGet:
		list.Logger.InfoContext(r.Context(), "Serving Template")
		lp := filepath.Join("dynamic", "layout.html")

		pageData := todoPageData{
			PageTitle: "TO DO LIST FOR " + uid,
		}
		list.Logger.InfoContext(r.Context(), "Getting user data")
		itemList := list.GetUserList(uid)

		pageData.Items = list.SortedArray(itemList)
		list.Logger.InfoContext(r.Context(), "Parse files")
		tmpl, err := template.New("layout.html").ParseFiles(lp)
		if err != nil {
			list.Logger.ErrorContext(r.Context(), "error parsing list template")
			return
		}
		err = tmpl.Execute(w, pageData)
		if err != nil {
			list.Logger.ErrorContext(r.Context(), "error executing list template")
		}

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
})

func main() {
	ctx := context.Background()

	err := list.BasicLoadToDoList()
	if err != nil {
		list.Logger.ErrorContext(ctx, "Error Loading todo List", "details", err)
		return
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Printf("\nclosing down...\n")
		err := list.BasicPersistEntries()

		if err != nil {
			list.Logger.ErrorContext(ctx, "Error saving todo List", "details", err)
		}
		os.Exit(1)
	}()

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./static"))

	mux.Handle("/todo", TracingMiddleware(ProcessRequestWithoutActor))
	mux.Handle("/todo/", http.StripPrefix("/todo/", fs))
	fmt.Printf("\nListening on port 8000\n")
	if err := http.ListenAndServe(":8000", mux); err != nil {
		fmt.Printf("error running http server: %s\n", err)
	}
}

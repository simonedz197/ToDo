package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	_ "net/http/pprof"

	"github.com/google/uuid"
	list "github.com/simonedz197/ToDoListStore"
)

var portFlag = flag.String("port", "", "port to run on e.g. -port 8080")

type RequestJob struct {
	Writer  http.ResponseWriter
	Request *http.Request
	uid     string
	done    chan struct{}
}

type RequetHeaderKey string

type IdRequestHeader string

var Queue = make(chan RequestJob)

type todoPageData struct {
	PageTitle string
	Items     []list.ToDoItem
}

func TracingMiddleware(next http.Handler) http.Handler {
	key := IdRequestHeader("X-Request-ID")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestId := r.Header.Get(string(key))
		if requestId == "" {
			requestId = uuid.NewString()
		}
		ctx := context.WithValue(r.Context(), key, requestId)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func postRequest(job RequestJob) {
	defer func() {
		close(job.done)
	}()

	var pb = make(map[string]string)
	err := json.NewDecoder(job.Request.Body).Decode(&pb)
	if err != nil {
		message := fmt.Sprintf("error decoding data data %v", err)
		LogThis(job.Request.Context(), list.ErrorLog, message)
		http.Error(job.Writer, err.Error(), http.StatusBadRequest)
		return
	}
	data := list.DataStoreJob{Context: job.Request.Context(), Uid: job.uid, JobType: list.AddData, KeyValue: pb["item"], AltValue: "", ReturnChannel: make(chan list.ReturnChannelData)}
	list.DataJobQueue <- data
	returnVal, ok := <-data.ReturnChannel
	if ok {
		if returnVal.Err != nil {
			message := fmt.Sprintf("error adding data data %v", err)
			LogThis(job.Request.Context(), list.ErrorLog, message)
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
		message := fmt.Sprintf("error decoding data data %v", err)
		LogThis(job.Request.Context(), list.ErrorLog, message)
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
			message := fmt.Sprintf("error updating data data %v", returnVal.Err)
			LogThis(job.Request.Context(), list.ErrorLog, message)
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
		message := fmt.Sprintf("error decoding data data %v", err)
		LogThis(job.Request.Context(), list.ErrorLog, message)
		job.Writer.WriteHeader(http.StatusBadRequest)
		return
	}
	data := list.DataStoreJob{Context: job.Request.Context(), Uid: job.uid, JobType: list.DeleteData, KeyValue: db["item"], AltValue: "", ReturnChannel: make(chan list.ReturnChannelData)}
	list.DataJobQueue <- data
	returnVal, ok := <-data.ReturnChannel
	if ok {
		if returnVal.Err != nil {
			message := fmt.Sprintf("error deleting data %v", returnVal.Err)
			LogThis(job.Request.Context(), list.ErrorLog, message)
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
	lp := filepath.Join("dynamic", "layout.html")

	pageData := todoPageData{
		PageTitle: "TO DO LIST FOR " + job.uid,
	}

	data := list.DataStoreJob{Context: job.Request.Context(), Uid: job.uid, JobType: list.FetchData, KeyValue: "", AltValue: "", ReturnChannel: make(chan list.ReturnChannelData)}
	list.DataJobQueue <- data
	returnVal, ok := <-data.ReturnChannel
	if ok {
		if returnVal.Err != nil {
			message := fmt.Sprintf("error fetching data %v", returnVal.Err)
			LogThis(job.Request.Context(), list.ErrorLog, message)
			job.Writer.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	pageData.Items = list.SortedArray(returnVal.List)

	tmpl, err := template.New("layout.html").ParseFiles(lp)
	if err != nil {
		message := fmt.Sprintf("error parsing list template %v", err)
		LogThis(job.Request.Context(), list.ErrorLog, message)
		return
	}
	err = tmpl.Execute(job.Writer, pageData)
	if err != nil {
		message := fmt.Sprintf("error executing list template %v", err)
		LogThis(job.Request.Context(), list.ErrorLog, message)
	}
}

func ProcessHttpQueue() {
	for v := range Queue {
		message := fmt.Sprintf("Processing %s Request for %s", v.Request.Method, v.Request.RequestURI)
		LogThis(v.Request.Context(), list.InfoLog, message)
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
	if err == nil {
		uid = r.FormValue("uid")
	}
	data := RequestJob{w, r, uid, make(chan struct{})}
	Queue <- data
	<-data.done
})

func LogThis(ctx context.Context, level list.LogType, message string) {
	data := list.LoggerJob{Context: ctx, LogMessage: message, LogType: level}
	list.LoggerJobQueue <- data
}

func main() {

	flag.Parse()
	port := fmt.Sprintf(":%s", *portFlag)
	filename := fmt.Sprintf("todo%s.txt", port)

	ctx := context.Background()
	go ProcessHttpQueue()
	go list.ProcessLoggerJobs()
	go list.ProcessDataJobs()

	data := list.DataStoreJob{Context: ctx, Uid: "", JobType: list.LoadData, KeyValue: filename, AltValue: "", ReturnChannel: make(chan list.ReturnChannelData)}
	list.DataJobQueue <- data
	returnVal, ok := <-data.ReturnChannel
	if ok {
		if returnVal.Err != nil {
			message := fmt.Sprintf("Error Loading todo List %v", returnVal.Err)
			LogThis(ctx, list.ErrorLog, message)
			return
		}
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Printf("\nclosing down...\n")

		data := list.DataStoreJob{Context: ctx, Uid: "", JobType: list.StoreData, KeyValue: filename, AltValue: "", ReturnChannel: make(chan list.ReturnChannelData)}
		list.DataJobQueue <- data
		returnVal, ok := <-data.ReturnChannel
		if ok {
			if returnVal.Err != nil {
				message := fmt.Sprintf("Error saving todo List %v", returnVal.Err)
				LogThis(ctx, list.ErrorLog, message)
				return
			}
		}
		os.Exit(1)
	}()

	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/debug/", http.DefaultServeMux)
	mux.Handle("/todo", TracingMiddleware(ProcessRequest))
	mux.Handle("/todo/", http.StripPrefix("/todo/", fs))

	fmt.Printf("\nListening on port %s\n", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		fmt.Printf("error running http server: %s\n", err)
	}
}

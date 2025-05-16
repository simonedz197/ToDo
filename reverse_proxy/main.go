package main

import (
	"crypto/md5"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
)

// create a server
func main() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Printf("\nclosing down...\n")
		os.Exit(1)
	}()

	mux := http.NewServeMux()
	mux.Handle("/", ProcessRequest)

	fmt.Printf("\nListening on port 8000\n")
	if err := http.ListenAndServe(":8000", mux); err != nil {
		fmt.Printf("error running http server: %s\n", err)
	}
}

// get a request

var ProcessRequest = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// parse the form to get the userid
	uid := "Anonymous User"
	err := r.ParseForm()

	if err == nil {
		uid = r.FormValue("uid")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	// implement ring hash

	data := []byte(uid)
	x := fmt.Sprintf("%x", md5.Sum(data))

	requestURL := fmt.Sprintf("http://localhost:8001")

	switch x[:1] {
	case "5", "6", "7", "8", "9":
		requestURL = "http://localhost:8002"
	case "a", "b", "c", "d", "e:", "f":
		requestURL = "http://localhost:8003"
	default:
		requestURL = "http://localhost:8001"
	}
	// forward request to one of servers on 8001, 8002 or 8003
	w.Header().Add("X-Forwarded-Server", requestURL)
	fmt.Printf("%s\n", requestURL)

	proxy, _ := NewProxy(requestURL)
	proxy.ServeHTTP(w, r)
})

// Returns a *httputil.ReverseProxy for the given target URL
func NewProxy(targetUrl string) (*httputil.ReverseProxy, error) {
	target, err := url.Parse(targetUrl)
	if err != nil {
		return nil, err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = func(response *http.Response) error {
		dumpedResponse, err := httputil.DumpResponse(response, false)
		if err != nil {
			return err
		}
		log.Println("Response: \r\n", string(dumpedResponse))
		return nil
	}
	return proxy, nil
}

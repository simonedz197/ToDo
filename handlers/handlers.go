package handlers

const BaseURLPath = "/todo"

func ServePing(writer http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	writer.Header().Set("Content-Type", "text/plain charset=utf-8")

	_, err := writer.Write([]byte("pong"))
	if err != nil {
		warnMessage := fmt.Sprintf("Could not write to response %v", err)
		writer.WriteHeader(http.StatusInternalServerError)
	}
}

// ShutdownServerChannel channel to monitor for shutting down the server.
var ShutdownServerChannel = make(chan int)

func ServeShutdown(writer http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	writer.Header().Set("Content-Type", "text/plain charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte("OK"))

	ShutdownServerChannel <- 1
}

func ServeList(writer http.ResponseWriter, req *http.Request) {
	// see if we have an additional key value in url
	// otherwise just return complete list
	path, _ := strings.CutPrefix(req.URL.Path, "/")
	path, _ = strings.CutSuffix(path, "/")
	elements := strings.Split(path, "/")

	if len(elements) > elementLimit {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	var (
		data  []byte
		count int
	)

	if len(elements) == 1 {
		// get complete list
		data, count = getList(username)
	} else {
		// get for specific key

		data, count = getListForKey(elements[1], username)

		if count == 0 {
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte("404 Key Not Found"))

			return
		}
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)

	if count > 0 {
		_, _ = writer.Write(data)
	} else {
		_, _ = writer.Write([]byte("[]"))
	}
}

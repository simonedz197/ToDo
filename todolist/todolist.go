package todolist

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
)

var mToDoList = make(map[int]string)

var NotFoundErr = fmt.Errorf("not found")
var AlreadyExistsErr = fmt.Errorf("already exists")

var logFile, err = os.OpenFile("todo.log", os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
var baseHandler = slog.NewTextHandler(logFile, &slog.HandlerOptions{AddSource: true})
var customHandler = &ContextHandler{Handler: baseHandler}
var Logger = slog.New(customHandler)

func init() {
	slog.SetDefault(Logger)
}

type ContextHandler struct {
	slog.Handler
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if traceid, ok := ctx.Value("X-Request-ID").(string); ok {
		r.AddAttrs(slog.String("trace_id", traceid))
	}
	if userID, ok := ctx.Value("user_id").(string); ok {
		r.AddAttrs(slog.String("user_id", userID))
	}
	return h.Handler.Handle(ctx, r)
}

type ToDoItem struct {
	Id   int
	Item string
}

type JobType int

const (
	LoadData = iota
	FetchData
	AddData
	UpdateData
	DeleteData
	StoreData
)

type ReturnChannelData struct {
	List map[int]string
	Err  error
}

type DataStoreJob struct {
	Context       context.Context
	JobType       JobType
	KeyValue      string
	AltValue      string
	ReturnChannel chan ReturnChannelData
}

var DataJobQueue = make(chan DataStoreJob, 100)

func ProcessDataJobs() {
	for v := range DataJobQueue {
		switch v.JobType {
		case LoadData:
			go LoadToDoList(v)
		case FetchData:
			go FetchToDoList(v)
		case AddData:
			go AddToDoItem(v)
		case UpdateData:
			go UpdateToDoItem(v)
		case DeleteData:
			go DeleteToDoItem(v)
		case StoreData:
			PersistEntries(v)
		}
	}
}

func LoadToDoList(dataJob DataStoreJob) {
	defer close(dataJob.ReturnChannel)
	returnChannelValue := ReturnChannelData{nil, nil}

	file, err := os.OpenFile("todo.txt", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		Logger.ErrorContext(dataJob.Context, fmt.Sprintf("error %v opening todo file", err))
		returnChannelValue.Err = err
	}
	defer file.Close()
	scan1 := bufio.NewScanner(file)
	for scan1.Scan() {
		if s := scan1.Text(); s != "" {
			index := getNewKey()
			mToDoList[index] = s
		}
	}
	returnChannelValue.List = mToDoList
	dataJob.ReturnChannel <- returnChannelValue
	return
}

func AddToDoItem(dataJob DataStoreJob) {
	defer close(dataJob.ReturnChannel)
	returnChannelData := ReturnChannelData{nil, nil}

	idx := itemExists(dataJob.KeyValue)

	if idx != -1 {
		returnChannelData.Err = AlreadyExistsErr
		dataJob.ReturnChannel <- returnChannelData
		return
	}
	idx = getNewKey()
	mToDoList[idx] = dataJob.KeyValue
	returnChannelData.List = mToDoList
	dataJob.ReturnChannel <- returnChannelData
	return
}

func UpdateToDoItem(dataJob DataStoreJob) {
	defer close(dataJob.ReturnChannel)
	returnChannelData := ReturnChannelData{nil, nil}
	idx := itemExists(dataJob.KeyValue)
	if idx == -1 {
		returnChannelData.Err = NotFoundErr
		dataJob.ReturnChannel <- returnChannelData
		return
	}
	mToDoList[idx] = dataJob.AltValue
	returnChannelData.List = mToDoList
	dataJob.ReturnChannel <- returnChannelData
	return
}

func DeleteToDoItem(dataJob DataStoreJob) {
	defer close(dataJob.ReturnChannel)
	returnChannelData := ReturnChannelData{nil, nil}
	if dataJob.KeyValue == "*" {
		// remove all items by just recreating the map
		mToDoList = make(map[int]string)
		returnChannelData.List = mToDoList
		return
	}

	idx := itemExists(dataJob.KeyValue)
	if idx == -1 {
		returnChannelData.Err = NotFoundErr
		dataJob.ReturnChannel <- returnChannelData
		return
	}

	delete(mToDoList, idx)
	returnChannelData.List = mToDoList
	return
}

func FetchToDoList(dataJob DataStoreJob) {
	defer close(dataJob.ReturnChannel)
	returnChannelData := ReturnChannelData{mToDoList, nil}
	dataJob.ReturnChannel <- returnChannelData
}

// func ToDoList() []string {
// 	list := make([]string, 0)
// 	for _, v := range mToDoList {
// 		list = append(list, v)
// 	}
// 	return list
// }

func SortedMap() []ToDoItem {

	sortedmap := make([]ToDoItem, 0)

	keys := make([]int, 0, len(mToDoList))
	for idx, _ := range mToDoList {
		keys = append(keys, idx)
	}
	sort.Ints(keys)
	index := 1
	for _, v := range keys {
		item := ToDoItem{index, mToDoList[v]}
		sortedmap = append(sortedmap, item)
		index += 1
	}
	return sortedmap
}

// only used locally so make private
func PersistEntries(dataJob DataStoreJob) {
	defer close(dataJob.ReturnChannel)
	returnChannelData := ReturnChannelData{nil, nil}
	file, err := os.Create("todo.txt")
	if err != nil {
		returnChannelData.Err = err
		dataJob.ReturnChannel <- returnChannelData
		return
	}
	defer file.Close()
	if len(mToDoList) > 0 {
		for _, v := range SortedMap() {
			_, err := file.WriteString(v.Item + "\n")
			if err != nil {
				returnChannelData.Err = err
				dataJob.ReturnChannel <- returnChannelData
				return
			}
		}
	}
	returnChannelData.List = mToDoList
	dataJob.ReturnChannel <- returnChannelData
	return
}

func getNewKey() int {
	keyVal := 0
	for idx, _ := range mToDoList {
		if idx > keyVal {
			keyVal = idx
		}
	}
	return keyVal + 1
}

func itemExists(searchString string) int {
	returnVal := -1
	for idx, val := range mToDoList {
		if val == searchString {
			returnVal = idx
			break
		}
	}
	return returnVal
}

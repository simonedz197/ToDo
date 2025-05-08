package ToDoListStore

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
)

type ToDoItem struct {
	Id   int
	Item string
}

type baseToDoList map[int]string

var UserToDoList = make(map[string]baseToDoList)

//var mToDoList = make(map[int]string)

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
	Uid           string
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

func getUserList(uid string) map[int]string {
	userlist, found := UserToDoList[uid]
	if !found {
		userlist = make(map[int]string)
	}
	return userlist
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
			line := strings.Split(s, ",")
			if len(line) == 2 {
				uid := line[0]
				userlist := getUserList(uid)
				index := getNewKey(userlist)
				userlist[index] = line[1]
				UserToDoList[uid] = userlist
			}
		}
	}
	returnChannelValue.List = UserToDoList[dataJob.Uid]
	dataJob.ReturnChannel <- returnChannelValue
	return
}

func AddToDoItem(dataJob DataStoreJob) {
	defer close(dataJob.ReturnChannel)
	returnChannelData := ReturnChannelData{nil, nil}

	userlist := getUserList(dataJob.Uid)

	idx := itemExists(userlist, dataJob.KeyValue)

	if idx != -1 {
		returnChannelData.Err = AlreadyExistsErr
		dataJob.ReturnChannel <- returnChannelData
		return
	}

	idx = getNewKey(userlist)
	userlist[idx] = dataJob.KeyValue
	UserToDoList[dataJob.Uid] = userlist
	returnChannelData.List = UserToDoList[dataJob.Uid]
	dataJob.ReturnChannel <- returnChannelData
	return
}

func UpdateToDoItem(dataJob DataStoreJob) {
	defer close(dataJob.ReturnChannel)
	returnChannelData := ReturnChannelData{nil, nil}

	userlist := getUserList(dataJob.Uid)

	idx := itemExists(userlist, dataJob.KeyValue)
	if idx == -1 {
		returnChannelData.Err = NotFoundErr
		dataJob.ReturnChannel <- returnChannelData
		return
	}
	userlist[idx] = dataJob.AltValue
	UserToDoList[dataJob.Uid] = userlist
	returnChannelData.List = userlist
	dataJob.ReturnChannel <- returnChannelData
	return
}

func DeleteToDoItem(dataJob DataStoreJob) {
	defer close(dataJob.ReturnChannel)
	returnChannelData := ReturnChannelData{nil, nil}

	userlist := getUserList(dataJob.Uid)

	if dataJob.KeyValue == "*" {
		// remove all items by just recreating the map
		userlist = make(map[int]string)
		UserToDoList[dataJob.Uid] = userlist
		returnChannelData.List = userlist
		return
	}

	idx := itemExists(userlist, dataJob.KeyValue)
	if idx == -1 {
		returnChannelData.Err = NotFoundErr
		dataJob.ReturnChannel <- returnChannelData
		return
	}

	delete(userlist, idx)
	UserToDoList[dataJob.Uid] = userlist
	returnChannelData.List = userlist
	return
}

func FetchToDoList(dataJob DataStoreJob) {
	defer close(dataJob.ReturnChannel)
	returnChannelData := ReturnChannelData{UserToDoList[dataJob.Uid], nil}
	dataJob.ReturnChannel <- returnChannelData
}

func SortedMap(userlist map[int]string) []ToDoItem {

	sortedmap := make([]ToDoItem, 0)

	keys := make([]int, 0, len(userlist))
	for idx, _ := range userlist {
		keys = append(keys, idx)
	}
	sort.Ints(keys)
	index := 1
	for _, v := range keys {
		item := ToDoItem{index, userlist[v]}
		sortedmap = append(sortedmap, item)
		index += 1
	}
	return sortedmap
}

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
	if len(UserToDoList) > 0 {
		for i, u := range UserToDoList {
			for _, v := range SortedMap(u) {
				_, err := file.WriteString(i + "," + v.Item + "\n")
				if err != nil {
					returnChannelData.Err = err
					dataJob.ReturnChannel <- returnChannelData
					return
				}
			}
		}
	}
	returnChannelData.List = nil
	dataJob.ReturnChannel <- returnChannelData
	return
}

func getNewKey(userlist map[int]string) int {
	keyVal := 0
	for idx, _ := range userlist {
		if idx > keyVal {
			keyVal = idx
		}
	}
	return keyVal + 1
}

func itemExists(userlist map[int]string, searchString string) int {
	returnVal := -1
	for idx, val := range userlist {
		if val == searchString {
			returnVal = idx
			break
		}
	}
	return returnVal
}

func SortedArray(userlist map[int]string) []ToDoItem {
	returnVal := make([]ToDoItem, 0)
	keys := make([]int, 0, len(userlist))
	for idx, _ := range userlist {
		keys = append(keys, idx)
	}
	sort.Ints(keys)
	index := 1
	for _, v := range keys {
		item := ToDoItem{index, userlist[v]}
		returnVal = append(returnVal, item)
		index += 1
	}
	return returnVal
}

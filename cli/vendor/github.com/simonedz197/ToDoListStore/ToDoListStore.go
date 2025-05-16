package ToDoListStore

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type ToDoItem struct {
	Id   int
	Item string
}

type baseToDoList map[int]string

var mutex sync.Mutex
var UserToDoList = make(map[string]baseToDoList)

//var mToDoList = make(map[int]string)

var NotFoundErr = fmt.Errorf("not found")
var AlreadyExistsErr = fmt.Errorf("already exists")

var logFilename = fmt.Sprintf("todo-%d.log", time.Now().UnixMicro())
var logFile, err = os.OpenFile(logFilename, os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
var baseHandler = slog.NewTextHandler(logFile, &slog.HandlerOptions{AddSource: true})
var customHandler = &ContextHandler{Handler: baseHandler}
var Logger = slog.New(customHandler)

func Init() {
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
type LogType int

const (
	LoadData = iota
	FetchData
	AddData
	UpdateData
	DeleteData
	StoreData
)

const (
	InfoLog  = 1
	ErrorLog = 2
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

type LoggerJob struct {
	LogType    LogType
	Context    context.Context
	LogMessage string
}

var DataJobQueue = make(chan DataStoreJob, 1000)
var LoggerJobQueue = make(chan LoggerJob, 1000)

func ProcessDataJobs() {
	for v := range DataJobQueue {
		switch v.JobType {
		case LoadData:
			LoadToDoList(v)
		case FetchData:
			FetchToDoList(v)
		case AddData:
			AddToDoItem(v)
		case UpdateData:
			UpdateToDoItem(v)
		case DeleteData:
			DeleteToDoItem(v)
		case StoreData:
			PersistEntries(v)
		}
	}
}

func ProcessLoggerJobs() {
	for v := range LoggerJobQueue {
		switch v.LogType {
		case InfoLog:
			Logger.InfoContext(v.Context, v.LogMessage)
		case ErrorLog:
			Logger.ErrorContext(v.Context, v.LogMessage)
		default:
			Logger.InfoContext(v.Context, v.LogMessage)
		}
	}
}

func GetUserList(uid string) map[int]string {
	userlist, found := UserToDoList[uid]
	if !found {
		userlist = make(map[int]string)
	}
	return userlist
}

func LoadToDoList(dataJob DataStoreJob) {
	defer close(dataJob.ReturnChannel)
	returnChannelValue := ReturnChannelData{nil, nil}

	file, err := os.OpenFile(dataJob.KeyValue, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
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
				userlist := GetUserList(uid)
				index := getNewKey(userlist)
				userlist[index] = line[1]
				UserToDoList[uid] = userlist
			}
		}
	}
	returnChannelValue.List = UserToDoList[dataJob.Uid]
	dataJob.ReturnChannel <- returnChannelValue
}

func AddToDoItem(dataJob DataStoreJob) {
	defer close(dataJob.ReturnChannel)
	returnChannelData := ReturnChannelData{nil, nil}

	userlist := GetUserList(dataJob.Uid)

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
}

func UpdateToDoItem(dataJob DataStoreJob) {
	defer close(dataJob.ReturnChannel)
	returnChannelData := ReturnChannelData{nil, nil}

	userlist := GetUserList(dataJob.Uid)

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
}

func DeleteToDoItem(dataJob DataStoreJob) {
	defer close(dataJob.ReturnChannel)
	returnChannelData := ReturnChannelData{nil, nil}

	userlist := GetUserList(dataJob.Uid)

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
}

func BasicLoadToDoList() error {
	mutex.Lock()

	defer func() {
		mutex.Unlock()
	}()

	file, err := os.OpenFile("todo.txt", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		Logger.ErrorContext(context.Background(), fmt.Sprintf("error %v opening todo file", err))
		return err
	}
	defer file.Close()
	scan1 := bufio.NewScanner(file)
	for scan1.Scan() {
		if s := scan1.Text(); s != "" {
			line := strings.Split(s, ",")
			if len(line) == 2 {
				uid := line[0]
				userlist := GetUserList(uid)
				index := getNewKey(userlist)
				userlist[index] = line[1]
				UserToDoList[uid] = userlist
			}
		}
	}
	return nil
}

func BasicPersistEntries() error {
	file, err := os.Create("todo.txt")
	if err != nil {
		return err
	}
	defer file.Close()
	if len(UserToDoList) > 0 {
		for i, u := range UserToDoList {
			for _, v := range SortedMap(u) {
				if v.Item != "" {
					_, err := file.WriteString(i + "," + v.Item + "\n")
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func BasicAddToDoItem(uid string, item string) error {
	mutex.Lock()

	defer func() {
		mutex.Unlock()
	}()

	userlist := GetUserList(uid)
	idx := itemExists(userlist, item)
	if idx != -1 {
		return AlreadyExistsErr
	}
	idx = getNewKey(userlist)
	userlist[idx] = item
	UserToDoList[uid] = userlist
	return nil
}

func BasicUpdateToDoItem(uid string, item string, replacewith string) error {
	mutex.Lock()

	defer func() {
		mutex.Unlock()
	}()

	userlist := GetUserList(uid)
	idx := itemExists(userlist, item)
	if idx == -1 {
		return NotFoundErr
	}
	userlist[idx] = replacewith
	UserToDoList[uid] = userlist
	return nil
}

func BasicDeleteToDoItem(uid string, item string) error {
	mutex.Lock()

	defer func() {
		mutex.Unlock()
	}()

	userlist := GetUserList(uid)

	if item == "*" {
		// remove all items by just recreating the map
		userlist = make(map[int]string)
		UserToDoList[uid] = userlist
		return nil
	}

	idx := itemExists(userlist, item)
	if idx == -1 {
		return NotFoundErr
	}

	delete(userlist, idx)
	UserToDoList[uid] = userlist
	return nil
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
	file, err := os.Create(dataJob.KeyValue)
	if err != nil {
		returnChannelData.Err = err
		dataJob.ReturnChannel <- returnChannelData
		return
	}
	defer file.Close()
	if len(UserToDoList) > 0 {
		for i, u := range UserToDoList {
			for _, v := range SortedMap(u) {
				if v.Item != "" {
					_, err := file.WriteString(i + "," + v.Item + "\n")
					if err != nil {
						returnChannelData.Err = err
						dataJob.ReturnChannel <- returnChannelData
						return
					}
				}
			}
		}
	}
	returnChannelData.List = nil
	dataJob.ReturnChannel <- returnChannelData
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

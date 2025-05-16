package main

import (
	"context"
	"flag"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/uuid"
	list "github.com/simonedz197/ToDoListStore"
)

var uidFlag = flag.String("uid", "", "owner of the todo list e.g. -uid simon")
var addFlag = flag.String("add", "", "add the todo list entry e.g. -add \"buy milk\"")
var updateFlag = flag.String("update", "", "update the todo list entry e.g. -update \"buy milk\" \"buy 2 pints of milk\"")
var deleteFlag = flag.String("delete", "", "delete the todo list entry e.g. -delete \"buy milk\"\nUse delete \"*\" to delete all")

type RequestId string
type UserId string

// create a dummy context
func dummyContext() context.Context {
	request_id := RequestId(uuid.NewString())
	user_id := UserId("edz197")

	ctx := context.Background()
	ctx = context.WithValue(ctx, reflect.TypeOf(request_id), request_id)
	ctx = context.WithValue(ctx, reflect.TypeOf(user_id), user_id)
	return ctx
}

// return names of all flags passed in
// we are hoping there is only 1
func flagsPassed() []string {
	name := ""
	flag.Visit(func(f *flag.Flag) {
		if f.Name != "uid" {
			name += f.Name + "|"
		}
	})
	// if no flags default to list
	if name == "" {
		name = "list|"
	}
	return strings.Split(name[:len(name)-1], "|")
}

func main() {
	// setup a dummy context
	// we should get this passed in eventually
	ctx := dummyContext()

	// start the job queue prcessor
	go list.ProcessDataJobs()

	flag.Parse()

	flagsSet := flagsPassed()

	if len(flagsSet) > 2 {
		list.Logger.ErrorContext(ctx, "Error parsing command line", "details", "too many flags passed")
		return
	}
	// load data
	data := list.DataStoreJob{Context: ctx, Uid: "", JobType: list.LoadData, KeyValue: "todo.txt", AltValue: "", ReturnChannel: make(chan list.ReturnChannelData)}
	list.DataJobQueue <- data
	returnVal, ok := <-data.ReturnChannel
	if ok {
		if returnVal.Err != nil {
			list.Logger.ErrorContext(ctx, "Error Loading todo List", "details", returnVal.Err)
			return
		}
	}

	// save data deferred to last thing to do
	defer func() {
		fmt.Printf("\nclosing down...\n")
		data := list.DataStoreJob{Context: ctx, Uid: "", JobType: list.StoreData, KeyValue: "todo.txt", AltValue: "", ReturnChannel: make(chan list.ReturnChannelData)}
		list.DataJobQueue <- data
		returnVal, ok := <-data.ReturnChannel
		if ok {
			if returnVal.Err != nil {
				list.Logger.ErrorContext(ctx, "Error saving todo List", "details", returnVal.Err)
				return
			}
		}
	}()

	switch flagsSet[0] {
	case "add":
		data := list.DataStoreJob{Context: ctx, Uid: *uidFlag, JobType: list.AddData, KeyValue: *addFlag, AltValue: "", ReturnChannel: make(chan list.ReturnChannelData)}
		list.DataJobQueue <- data
		returnVal, ok := <-data.ReturnChannel
		if ok {
			if returnVal.Err != nil {
				list.Logger.ErrorContext(ctx, "Error Adding to do item to list", "details", returnVal.Err)
				return
			}
		}
	case "delete":
		data := list.DataStoreJob{Context: ctx, Uid: *uidFlag, JobType: list.DeleteData, KeyValue: *deleteFlag, AltValue: "", ReturnChannel: make(chan list.ReturnChannelData)}
		list.DataJobQueue <- data
		returnVal, ok := <-data.ReturnChannel
		if ok {
			if returnVal.Err != nil {
				list.Logger.ErrorContext(ctx, "Error Deleting to do item from list", "details", returnVal.Err)
				return
			}
		}
	case "update":
		if flag.NArg() == 0 {
			fmt.Printf("\nyou need to enter the value to update to")
		}
		data := list.DataStoreJob{Context: ctx, Uid: *uidFlag, JobType: list.UpdateData, KeyValue: *updateFlag, AltValue: flag.Arg(0), ReturnChannel: make(chan list.ReturnChannelData)}
		list.DataJobQueue <- data
		returnVal, ok := <-data.ReturnChannel
		if ok {
			if returnVal.Err != nil {
				list.Logger.ErrorContext(ctx, "Error Deleting to do item from list", "details", returnVal.Err)
				return
			}
		}
	}
	data = list.DataStoreJob{Context: ctx, Uid: *uidFlag, JobType: list.FetchData, KeyValue: "", AltValue: "", ReturnChannel: make(chan list.ReturnChannelData)}
	list.DataJobQueue <- data
	returnVal, ok = <-data.ReturnChannel
	if ok {
		if returnVal.Err != nil {
			list.Logger.ErrorContext(ctx, "Error listing to do items", "details", returnVal.Err)
			return
		}
		fmt.Printf("\nTO DO LIST\n----------\n")
		for _, v := range list.SortedArray(returnVal.List) {
			fmt.Printf("%d. %s\n", v.Id, v.Item)
		}
	}

}

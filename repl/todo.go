package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/google/uuid"
	list "github.com/simonedz197/ToDoListStore"
)

func dummyContext() context.Context {
	request_id := uuid.NewString()
	user_id := "edz197"

	ctx := context.Background()
	ctx = context.WithValue(ctx, "request_id", request_id)
	ctx = context.WithValue(ctx, "user_id", user_id)
	return ctx
}

func stripnl(s string) string {
	return strings.ToLower(s[:len(s)-1])
}

func main() {
	// setup a dummy context
	// we should get this passed in eventually
	ctx := dummyContext()

	// start the job queue prcessor
	go list.ProcessDataJobs()

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

	// monitor ctrl+c
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
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
		os.Exit(1)
	}()

	fmt.Printf("\nctrl+c to quit\n\n")

	for {
		// do this forever until ctrl+c is entered
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("\nEnter User Id :")
		uid, _ := reader.ReadString('\n')
		uid = stripnl(uid)
		if uid == "" {
			uid = "Anonympus User"
		}
		fmt.Printf("\nEnter Command (add/upd/del/lst) : ")
		cmd, _ := reader.ReadString('\n')
		cmd = stripnl(cmd)
		if cmd == "" {
			cmd = "lst"
		}

		item := ""
		replaceWith := ""

		switch cmd {
		case "add":
			fmt.Printf("\nEnter todo Item to %s : ", cmd)
			item, _ = reader.ReadString('\n')
			data := list.DataStoreJob{Context: ctx, Uid: uid, JobType: list.AddData, KeyValue: stripnl(item), AltValue: "", ReturnChannel: make(chan list.ReturnChannelData)}
			list.DataJobQueue <- data
			returnVal, ok := <-data.ReturnChannel
			if ok {
				if returnVal.Err != nil {
					list.Logger.ErrorContext(ctx, "Error Adding to do item to list", "details", returnVal.Err)
					fmt.Printf("\n\ncould not add. see log for details\n\n")
				}
			}
		case "del":
			fmt.Printf("\nEnter todo Item to %s : ", cmd)
			item, _ = reader.ReadString('\n')
			data := list.DataStoreJob{Context: ctx, Uid: uid, JobType: list.DeleteData, KeyValue: stripnl(item), AltValue: "", ReturnChannel: make(chan list.ReturnChannelData)}
			list.DataJobQueue <- data
			returnVal, ok := <-data.ReturnChannel
			if ok {
				if returnVal.Err != nil {
					list.Logger.ErrorContext(ctx, "Error Deleting to do item from list", "details", returnVal.Err)
					fmt.Printf("\n\ncould not delete. see log for details\n\n")
				}
			}
		case "upd":
			fmt.Printf("\nEnter todo Item to replace : ")
			item, _ = reader.ReadString('\n')
			fmt.Printf("\nnow enter todo item to replace with : ")
			replaceWith, _ = reader.ReadString('\n')
			data := list.DataStoreJob{Context: ctx, Uid: uid, JobType: list.UpdateData, KeyValue: stripnl(item), AltValue: stripnl(replaceWith), ReturnChannel: make(chan list.ReturnChannelData)}
			list.DataJobQueue <- data
			returnVal, ok := <-data.ReturnChannel
			if ok {
				if returnVal.Err != nil {
					list.Logger.ErrorContext(ctx, "Error Deleting to do item from list", "details", returnVal.Err)
					fmt.Printf("\n\ncould not update. see log for details\n\n")
				}
			}
		case "lst", "":
			data = list.DataStoreJob{Context: ctx, Uid: uid, JobType: list.FetchData, KeyValue: "", AltValue: "", ReturnChannel: make(chan list.ReturnChannelData)}
			list.DataJobQueue <- data
			returnVal, ok = <-data.ReturnChannel
			if ok {
				if returnVal.Err != nil {
					list.Logger.ErrorContext(ctx, "Error listing to do items", "details", returnVal.Err)
					return
				}
				fmt.Printf("\n%s TO DO LIST\n--------------------\n", uid)
				for _, v := range list.SortedArray(returnVal.List) {
					fmt.Printf("%d. %s\n", v.Id, v.Item)
				}
				fmt.Printf("--------------------\n\n")
			}
		case "quit":
			break
		default:
			list.Logger.ErrorContext(ctx, "Invalid command")
			fmt.Printf("\n\ninvalid command\n\n")
		}
	}
}

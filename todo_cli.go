package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/google/uuid"
	"log/slog"
	"os"
	"strings"
	list "tut2/todo/todolist"
)

var logFile, err = os.OpenFile("todo.log", os.O_APPEND|os.O_RDWR|os.O_CREATE, 0644)
var baseHandler = slog.NewTextHandler(logFile, &slog.HandlerOptions{AddSource: true})
var customHandler = &ContextHandler{Handler: baseHandler}
var logger = slog.New(customHandler)
var addFlag = flag.String("add", "", "add the todo list entry e.g. -add \"buy milk\"")
var updateFlag = flag.String("update", "", "update the todo list entry e.g. -update \"buy milk\" \"buy 2 pints of milk\"")
var deleteFlag = flag.String("delete", "", "delete the todo list entry e.g. -delete \"buy milk\"\nUse delete \"*\" to delete all")

func init() {
	slog.SetDefault(logger)
}

type ContextHandler struct {
	slog.Handler
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if requestID, ok := ctx.Value("request_id").(string); ok {
		r.AddAttrs(slog.String("request_id", requestID))
	}
	if userID, ok := ctx.Value("user_id").(string); ok {
		r.AddAttrs(slog.String("user_id", userID))
	}
	return h.Handler.Handle(ctx, r)
}

// create a dummy context
func dummyContext() context.Context {
	request_id := uuid.NewString()
	user_id := "edz197"

	ctx := context.Background()
	ctx = context.WithValue(ctx, "request_id", request_id)
	ctx = context.WithValue(ctx, "user_id", user_id)
	return ctx
}

// return names of all flags passed in
// we are hoping there is only 1
func flagsPassed() []string {
	name := ""
	flag.Visit(func(f *flag.Flag) {
		name += f.Name + "|"
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

	flag.Parse()

	flagsSet := flagsPassed()

	if len(flagsSet) > 1 {
		logger.ErrorContext(ctx, "Error parsing command line", "details", "too many flags passed")
		panic(errors.New("Error parsing command line too many flags"))
	}

	if err := list.LoadEntries(); err != nil {
		logger.ErrorContext(ctx, "Error Loading todo List", "details", err)
		panic(errors.New(fmt.Sprintf("Error Loading todo List %v", err)))
	}

	switch flagsSet[0] {
	case "add":
		if err := list.AddEntry(*addFlag); err != nil {
			logger.ErrorContext(ctx, "Error Adding to do item to list", "details", err)
		}
	case "delete":
		if err := list.DeleteEntry(*deleteFlag); err != nil {
			logger.ErrorContext(ctx, "Error Deleting to do item from list", "details", err)
		}
	case "update":
		if flag.NArg() == 0 {
			fmt.Printf("\nyou need to enter the value to update to")
		}
		if err := list.UpdateEntry(*updateFlag, flag.Arg(0)); err != nil {
			logger.ErrorContext(ctx, "Error Deleting to do item from list", "details", err)
		}
	}
	if err := list.ListEntries(); err != nil {
		logger.ErrorContext(ctx, "Error listing to do items", "details", err)
	}
}

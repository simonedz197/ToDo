package todolist

import (
	"bufio"
	"fmt"
	"os"
	"sort"
)

var mToDoList = make(map[int]string)
var NotFoundErr = fmt.Errorf("not found")
var AlreadyExistsErr = fmt.Errorf("already exists")

func LoadEntries() error {
	file, err := os.OpenFile("todo.txt", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	scan1 := bufio.NewScanner(file)
	for scan1.Scan() {
		if s := scan1.Text(); s != "" {
			index := getNewKey()
			mToDoList[index] = s
		}
	}
	return nil
}

func ToDoList() []string {
	list := make([]string, 0)
	for _, v := range mToDoList {
		list = append(list, v)
	}
	return list
}

func SortedMap() map[int]string {
	sortedmap := make(map[int]string)
	keys := make([]int, 0, len(mToDoList))
	for idx, _ := range mToDoList {
		keys = append(keys, idx)
	}
	sort.Ints(keys)
	index := 1
	for _, v := range keys {
		sortedmap[index] = mToDoList[v]
		index += 1
	}
	return sortedmap
}

// only used locally so make private
func persistEntries() error {
	file, err := os.Create("todo.txt")
	if err != nil {
		return err
	}
	defer file.Close()
	if len(mToDoList) > 0 {
		for _, v := range SortedMap() {
			_, err := file.WriteString(v + "\n")
			if err != nil {
				return err
			}
		}
	}
	return nil
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

func ListEntries() error {
	fmt.Println("Current ToDo list")
	if len(mToDoList) == 0 {
		fmt.Println("Nothing to do!")
	} else {
		for i, v := range SortedMap() {
			fmt.Printf("%0d. %s\n", i, v)
		}
	}
	return nil
}

func AddEntry(todoItem string) error {
	idx := ItemExists(todoItem)
	if idx == -1 {
		idx := getNewKey()
		mToDoList[idx] = todoItem
		if err := persistEntries(); err != nil {
			return err
		}
	} else {
		return AlreadyExistsErr
	}
	return nil
}

func UpdateEntry(oldTodoItem string, newTodoItem string) error {
	idx := ItemExists(oldTodoItem)
	if idx != -1 {
		mToDoList[idx] = newTodoItem
		if err := persistEntries(); err != nil {
			return err
		}
	} else {
		return NotFoundErr
	}
	return nil
}

func DeleteEntry(todoItem string) error {
	if todoItem == "*" {
		// remove all items by just recreating the map
		mToDoList = make(map[int]string)
		if err := persistEntries(); err != nil {
			return err
		}
	} else {
		idx := ItemExists(todoItem)
		if idx != -1 {
			delete(mToDoList, idx)
			if err := persistEntries(); err != nil {
				return err
			}
		} else {
			return NotFoundErr
		}
	}
	return nil
}

func ItemExists(searchString string) int {
	returnVal := -1
	for idx, val := range mToDoList {
		if val == searchString {
			returnVal = idx
			break
		}
	}
	return returnVal
}

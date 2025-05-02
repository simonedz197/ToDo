package todolist

import (
	"testing"
)

func TestLoadEntries(t *testing.T) {
	err := LoadEntries()
	if err != nil {
		t.Errorf("Expected nil got %v", err)
	}
}

func TestListEntries(t *testing.T) {
	err := ListEntries()
	if err != nil {
		t.Errorf("Expected nil got %v", err)
	}
}

func TestAddEntries(t *testing.T) {
	entry := "My Test Entry"

	err := AddEntry(entry)
	if err != nil {
		t.Errorf("Expected nil got %v", err)
	}
	if idx := ItemExists(entry); idx == -1 {
		t.Errorf("Expected !=-1 got %d", idx)
	}
}

func TestDeleteAll(t *testing.T) {
	err := DeleteEntry("*")
	if err != nil {
		t.Errorf("Expected nil got %v", err)
	}
	if size := len(mToDoList); size != 0 {
		t.Errorf("Expected 0 but got %d items", size)
	}
}

func TestDeleteEntry(t *testing.T) {
	entry := "My Delete Entry"
	if err := AddEntry(entry); err != nil {
		t.Errorf("Expected nil got %v adding entry", err)
	}
	if err := DeleteEntry(entry); err != nil {
		t.Errorf("Expected nil got %v deleting entry", err)
	}
}

func TestUpdateEntry(t *testing.T) {
	entry := "My Update Entry"
	newEntry := "My Updated Entry"
	if err := AddEntry(entry); err != nil {
		t.Errorf("Expected nil got %v adding entry", err)
	}
	if err := UpdateEntry(entry, newEntry); err != nil {
		t.Errorf("Expected nil got %v deleting entry", err)
	}
	if idx := ItemExists(newEntry); idx == -1 {
		t.Errorf("Expected and index for %s but got got -1", newEntry)
	}
}

func TestPersistEntriess(t *testing.T) {
	entry := "My Test Entry"
	if err := AddEntry(entry); err != nil {
		t.Errorf("Expected nil got %v adding entry", err)
	}
	if err := persistEntries(); err != nil {
		t.Errorf("Expected nil got %v persisting entries", err)
	}
}

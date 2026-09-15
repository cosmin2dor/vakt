package dispatch

import (
	"encoding/json"
	"testing"
)

func TestDefaultPayload(t *testing.T) {
	task := TaskContext{Title: "Water plants", FilePath: "Home/Chores.md"}

	raw, err := DefaultPayload(task)
	if err != nil {
		t.Fatalf("DefaultPayload returned error: %v", err)
	}

	var got payloadBody
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got.Title != "Water plants" {
		t.Errorf("Title = %q, want %q", got.Title, "Water plants")
	}
	if got.Body != "Due from /Home/Chores.md" {
		t.Errorf("Body = %q, want %q", got.Body, "Due from /Home/Chores.md")
	}
}

func TestDefaultPayload_EscapedCharacters(t *testing.T) {
	task := TaskContext{
		Title:    `Say "hi" \ to café`,
		FilePath: `Notes/"weird"\file 日本語.md`,
	}

	raw, err := DefaultPayload(task)
	if err != nil {
		t.Fatalf("DefaultPayload returned error: %v", err)
	}

	var got payloadBody
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got.Title != task.Title {
		t.Errorf("Title = %q, want %q", got.Title, task.Title)
	}
	want := "Due from /" + task.FilePath
	if got.Body != want {
		t.Errorf("Body = %q, want %q", got.Body, want)
	}
}

func TestDefaultPayload_EmptyFilePath(t *testing.T) {
	task := TaskContext{Title: "Untitled", FilePath: ""}

	raw, err := DefaultPayload(task)
	if err != nil {
		t.Fatalf("DefaultPayload returned error: %v", err)
	}

	var got payloadBody
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	// No path known: body reads "Due from /" rather than omitting the field.
	if got.Body != "Due from /" {
		t.Errorf("Body = %q, want %q", got.Body, "Due from /")
	}
}

func TestDefaultPayload_LeadingSlashAlreadyPresent(t *testing.T) {
	task := TaskContext{Title: "Task", FilePath: "/Folder/File.md"}

	raw, err := DefaultPayload(task)
	if err != nil {
		t.Fatalf("DefaultPayload returned error: %v", err)
	}

	var got payloadBody
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got.Body != "Due from /Folder/File.md" {
		t.Errorf("Body = %q, want %q", got.Body, "Due from /Folder/File.md")
	}
}

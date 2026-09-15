package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cosmin2dor/vakt/internal/model"
)

// writeFile creates path (relative to dir) with content, making parent dirs.
func writeFile(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// nestedVault builds Household/Routines.md, Household/Chores/Kitchen.md,
// Notes.md, and a .git dotfile to prove it's excluded.
func nestedVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "Notes.md", "# Notes")
	writeFile(t, dir, "Household/Routines.md", "- [ ] Water plants @schedule(...)")
	writeFile(t, dir, "Household/Chores/Kitchen.md", "- [ ] Wash dishes")
	writeFile(t, dir, ".git/config", "[core]")
	return dir
}

func TestListDirectories_NestedVault(t *testing.T) {
	dir := nestedVault(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/directories", nil)
	rec := httptest.NewRecorder()
	ListDirectoriesHandler(dir)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var root model.DirectoryEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if root.Type != model.Directory {
		t.Fatalf("root type = %s, want directory", root.Type)
	}
	if root.Path != "." {
		t.Fatalf("root path = %q, want \".\"", root.Path)
	}
	if root.Children == nil {
		t.Fatal("root children is nil")
	}
	children := *root.Children
	if len(children) != 2 { // Household dir + Notes.md; .git excluded
		t.Fatalf("root has %d children, want 2: %+v", len(children), children)
	}

	// directories sort before files: Household, then Notes.md
	household := children[0]
	if household.Name != "Household" || household.Type != model.Directory {
		t.Fatalf("children[0] = %+v, want Household directory", household)
	}
	notes := children[1]
	if notes.Name != "Notes.md" || notes.Type != model.File {
		t.Fatalf("children[1] = %+v, want Notes.md file", notes)
	}
	if notes.Children != nil {
		t.Fatalf("file entry has non-nil children: %+v", notes.Children)
	}

	householdChildren := *household.Children
	if len(householdChildren) != 2 { // Chores dir + Routines.md
		t.Fatalf("Household has %d children, want 2: %+v", len(householdChildren), householdChildren)
	}
	chores := householdChildren[0]
	if chores.Name != "Chores" || chores.Type != model.Directory {
		t.Fatalf("Household child[0] = %+v, want Chores directory", chores)
	}
	choresChildren := *chores.Children
	if len(choresChildren) != 1 || choresChildren[0].Name != "Kitchen.md" {
		t.Fatalf("Chores children = %+v, want [Kitchen.md]", choresChildren)
	}
}

func TestListDirectories_EmptyVault(t *testing.T) {
	dir := t.TempDir()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/directories", nil)
	rec := httptest.NewRecorder()
	ListDirectoriesHandler(dir)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var root model.DirectoryEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &root); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if root.Type != model.Directory {
		t.Fatalf("root type = %s, want directory", root.Type)
	}
	if root.Children == nil || len(*root.Children) != 0 {
		t.Fatalf("root children = %v, want non-nil empty slice", root.Children)
	}
}

func TestGetFile_Success(t *testing.T) {
	dir := nestedVault(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files?path=Household/Routines.md", nil)
	rec := httptest.NewRecorder()
	GetFileHandler(dir)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var fc model.FileContent
	if err := json.Unmarshal(rec.Body.Bytes(), &fc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := "- [ ] Water plants @schedule(...)"
	if fc.Content != want {
		t.Fatalf("content = %q, want %q", fc.Content, want)
	}
	if fc.Size != len(want) {
		t.Fatalf("size = %d, want %d", fc.Size, len(want))
	}
	if fc.Path != "Household/Routines.md" {
		t.Fatalf("path = %q, want vault-relative query path", fc.Path)
	}
	if fc.ModifiedAt == nil {
		t.Fatal("modified_at is nil")
	}
}

func TestGetFile_MissingPathParam(t *testing.T) {
	dir := nestedVault(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)
	rec := httptest.NewRecorder()
	GetFileHandler(dir)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetFile_NonexistentFile(t *testing.T) {
	dir := nestedVault(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files?path=Nope.md", nil)
	rec := httptest.NewRecorder()
	GetFileHandler(dir)(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetFile_PathIsDirectory(t *testing.T) {
	dir := nestedVault(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files?path=Household", nil)
	rec := httptest.NewRecorder()
	GetFileHandler(dir)(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetFile_TraversalRejected(t *testing.T) {
	// Place a real file just outside the vault root and prove it's unreachable.
	parent := t.TempDir()
	vault := filepath.Join(parent, "vault")
	if err := os.Mkdir(vault, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	secret := filepath.Join(parent, "secret.txt")
	if err := os.WriteFile(secret, []byte("do not read me"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	cases := []string{
		"../secret.txt",
		"../../../../../../../../etc/passwd",
		"/etc/passwd",
	}
	for _, path := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/files?path="+path, nil)
		req.URL.RawQuery = "path=" + path // avoid httptest re-encoding surprises
		rec := httptest.NewRecorder()
		GetFileHandler(vault)(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("path %q: status = %d, want 400, body = %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestResolveVaultPath(t *testing.T) {
	root := "/vault"
	cases := []struct {
		path string
		ok   bool
	}{
		{"Notes.md", true},
		{"Household/Routines.md", true},
		{"../vault-evil/secret.txt", false},
		{"../../etc/passwd", false},
		{"/etc/passwd", false},
	}
	for _, c := range cases {
		_, err := resolveVaultPath(root, c.path)
		ok := err == nil
		if ok != c.ok {
			t.Errorf("resolveVaultPath(%q, %q) ok = %v, want %v", root, c.path, ok, c.ok)
		}
	}
}

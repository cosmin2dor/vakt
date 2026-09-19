package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cosmin2dor/vakt/internal/engine/vault"
	"github.com/cosmin2dor/vakt/internal/model"
)

// ListDirectoriesHandler returns the whole vault as one tree rooted at
// vaultRoot (schema/openapi.yaml GET /directories).
func ListDirectoriesHandler(vaultRoot string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		root, err := buildTree(vaultRoot, "")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to walk vault: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, root)
	}
}

// buildTree recurses one directory. relPath is "" for the vault root itself;
// name mirrors the vault root's base directory name there, and the entry's
// path uses "." for root (there's no vault-relative path shorter than that).
func buildTree(vaultRoot, relPath string) (model.DirectoryEntry, error) {
	abs := filepath.Join(vaultRoot, relPath)
	entries, err := os.ReadDir(abs)
	if err != nil {
		return model.DirectoryEntry{}, err
	}

	children := make([]model.DirectoryEntry, 0, len(entries))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") { // hidden files/dirs (e.g. .git) aren't browsable vault content
			continue
		}
		childRel := e.Name()
		if relPath != "" {
			childRel = filepath.Join(relPath, e.Name())
		}
		if e.IsDir() {
			child, err := buildTree(vaultRoot, childRel)
			if err != nil {
				return model.DirectoryEntry{}, err
			}
			children = append(children, child)
		} else {
			children = append(children, model.DirectoryEntry{
				Name: e.Name(),
				Path: filepath.ToSlash(childRel),
				Type: model.File,
			})
		}
	}
	sort.Slice(children, func(i, j int) bool {
		ci, cj := children[i], children[j]
		if (ci.Type == model.Directory) != (cj.Type == model.Directory) {
			return ci.Type == model.Directory // directories before files
		}
		return ci.Name < cj.Name
	})

	name := filepath.Base(vaultRoot)
	path := "."
	if relPath != "" {
		name = filepath.Base(relPath)
		path = filepath.ToSlash(relPath)
	}
	return model.DirectoryEntry{
		Name:     name,
		Path:     path,
		Type:     model.Directory,
		Children: &children,
	}, nil
}

// GetFileHandler reads one vault file's raw content by vault-relative path
// (schema/openapi.yaml GET /files).
func GetFileHandler(vaultRoot string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		relPath := r.URL.Query().Get("path")
		if relPath == "" {
			writeError(w, http.StatusBadRequest, "invalid_path", "path query parameter is required")
			return
		}

		abs, err := resolveVaultPath(vaultRoot, relPath)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_path", "path escapes the vault root")
			return
		}

		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			writeError(w, http.StatusNotFound, "file_not_found", "no such file: "+relPath)
			return
		}

		content, err := os.ReadFile(abs)
		if err != nil {
			writeError(w, http.StatusNotFound, "file_not_found", "no such file: "+relPath)
			return
		}

		modTime := info.ModTime()
		writeJSON(w, http.StatusOK, model.FileContent{
			Path:       relPath,
			Content:    string(content),
			Size:       len(content),
			ModifiedAt: &modTime,
		})
	}
}

// PutFileHandler overwrites one vault file's whole content by vault-relative
// path (schema/openapi.yaml PUT /files). A full-file publish via
// vault.Writer.Write, not a directive patch — free-form editing touches
// arbitrary bytes, so there's no single span to patch. No conflict
// detection: a concurrent external edit is silently overwritten.
func PutFileHandler(vaultRoot string, writer *vault.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		relPath := r.URL.Query().Get("path")
		if relPath == "" {
			writeError(w, http.StatusBadRequest, "invalid_path", "path query parameter is required")
			return
		}

		abs, err := resolveVaultPath(vaultRoot, relPath)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_path", "path escapes the vault root")
			return
		}

		if info, err := os.Stat(abs); err != nil || info.IsDir() {
			writeError(w, http.StatusNotFound, "file_not_found", "no such file: "+relPath)
			return
		}

		var body model.FileWrite
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
			return
		}

		if _, err := writer.Write(abs, []byte(body.Content)); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "writing file: "+err.Error())
			return
		}

		info, err := os.Stat(abs)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "statting written file: "+err.Error())
			return
		}
		modTime := info.ModTime()
		writeJSON(w, http.StatusOK, model.FileContent{
			Path:       relPath,
			Content:    body.Content,
			Size:       len(body.Content),
			ModifiedAt: &modTime,
		})
	}
}

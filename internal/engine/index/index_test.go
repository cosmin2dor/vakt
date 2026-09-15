package index

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedVault(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
		require.NoError(t, os.WriteFile(abs, []byte(content), 0o644))
	}
	return root
}

func TestBuild_SeededVaultIndexesCorrectly(t *testing.T) {
	root := seedVault(t, map[string]string{
		"household.md": "- [ ] Feed the dog @id(dog_feed) @target(ios_notifications) @schedule(0 8 * * *) @state(active)\n",
	})

	idx := New(root, time.UTC)
	require.NoError(t, idx.Build())

	task, ok := idx.Task("dog_feed")
	require.True(t, ok)
	assert.Equal(t, "household.md", task.Path)
	assert.Equal(t, 1, task.Line)
	assert.Equal(t, "ios_notifications", task.Values["target"].Str)
	assert.Equal(t, "active", task.Values["state"].Str)
	assert.Equal(t, []string{"0", "8", "*", "*", "*"}, task.Values["schedule"].Cron)
	assert.Empty(t, idx.Diagnostics())
}

func TestBuild_MissingIDIsNotATaskButIsDiagnosed(t *testing.T) {
	root := seedVault(t, map[string]string{
		"household.md": "- [ ] No id here @state(active)\n",
	})

	idx := New(root, time.UTC)
	require.NoError(t, idx.Build())

	assert.Empty(t, idx.Tasks())
	diags := idx.Diagnostics()
	require.Len(t, diags, 1)
	assert.Equal(t, "missing_id", *diags[0].Code)
}

func TestBuild_InvalidIDIsNotATaskButIsDiagnosed(t *testing.T) {
	root := seedVault(t, map[string]string{
		"household.md": "- [ ] Bad id @id(Has Spaces)\n",
	})

	idx := New(root, time.UTC)
	require.NoError(t, idx.Build())

	assert.Empty(t, idx.Tasks())
	diags := idx.Diagnostics()
	require.Len(t, diags, 1)
	assert.Equal(t, "invalid_id", *diags[0].Code)
}

func TestBuild_CrossFileDuplicateFirstWinsByPath(t *testing.T) {
	root := seedVault(t, map[string]string{
		"a-first.md":  "- [ ] First claim @id(shared)\n",
		"b-second.md": "- [ ] Second claim @id(shared)\n",
	})

	idx := New(root, time.UTC)
	require.NoError(t, idx.Build())

	task, ok := idx.Task("shared")
	require.True(t, ok)
	assert.Equal(t, "a-first.md", task.Path, "first by path must win (G4)")

	diags := idx.Diagnostics()
	require.Len(t, diags, 1)
	assert.Equal(t, "duplicate_id", *diags[0].Code)
	assert.Equal(t, "b-second.md", diags[0].FilePath)
}

func TestReconcileFile_Added(t *testing.T) {
	root := seedVault(t, map[string]string{
		"household.md": "- [ ] Feed the dog @id(dog_feed)\n",
	})
	idx := New(root, time.UTC)
	require.NoError(t, idx.Build())

	path := filepath.Join(root, "household.md")
	newContent := "- [ ] Feed the dog @id(dog_feed)\n- [ ] Water plants @id(water_plants)\n"
	require.NoError(t, os.WriteFile(path, []byte(newContent), 0o644))

	change, err := idx.ReconcileFile("household.md")
	require.NoError(t, err)

	require.Len(t, change.Added, 1)
	assert.Equal(t, "water_plants", change.Added[0].ID)
	assert.Empty(t, change.Updated)
	assert.Empty(t, change.Removed)

	_, ok := idx.Task("water_plants")
	assert.True(t, ok)
}

func TestReconcileFile_Updated(t *testing.T) {
	root := seedVault(t, map[string]string{
		"household.md": "- [ ] Feed the dog @id(dog_feed) @state(active)\n",
	})
	idx := New(root, time.UTC)
	require.NoError(t, idx.Build())

	path := filepath.Join(root, "household.md")
	require.NoError(t, os.WriteFile(path, []byte("- [ ] Feed the dog @id(dog_feed) @state(paused)\n"), 0o644))

	change, err := idx.ReconcileFile("household.md")
	require.NoError(t, err)

	assert.Empty(t, change.Added)
	require.Len(t, change.Updated, 1)
	assert.Equal(t, "paused", change.Updated[0].Values["state"].Str)
	assert.Empty(t, change.Removed)

	task, _ := idx.Task("dog_feed")
	assert.Equal(t, "paused", task.Values["state"].Str)
}

func TestReconcileFile_Removed(t *testing.T) {
	root := seedVault(t, map[string]string{
		"household.md": "- [ ] Feed the dog @id(dog_feed)\n- [ ] Water plants @id(water_plants)\n",
	})
	idx := New(root, time.UTC)
	require.NoError(t, idx.Build())

	path := filepath.Join(root, "household.md")
	require.NoError(t, os.WriteFile(path, []byte("- [ ] Feed the dog @id(dog_feed)\n"), 0o644))

	change, err := idx.ReconcileFile("household.md")
	require.NoError(t, err)

	assert.Empty(t, change.Added)
	assert.Empty(t, change.Updated)
	require.Len(t, change.Removed, 1)
	assert.Equal(t, "water_plants", change.Removed[0])

	_, ok := idx.Task("water_plants")
	assert.False(t, ok)
	_, ok = idx.Task("dog_feed")
	assert.True(t, ok, "unrelated task on the same file must survive")
}

func TestReconcileFile_FileDeletedRemovesAllItsTasks(t *testing.T) {
	root := seedVault(t, map[string]string{
		"household.md": "- [ ] Feed the dog @id(dog_feed)\n",
		"other.md":     "- [ ] Unrelated @id(other_task)\n",
	})
	idx := New(root, time.UTC)
	require.NoError(t, idx.Build())

	require.NoError(t, os.Remove(filepath.Join(root, "household.md")))

	change, err := idx.ReconcileFile("household.md")
	require.NoError(t, err)

	assert.Empty(t, change.Added)
	assert.Empty(t, change.Updated)
	require.Len(t, change.Removed, 1)
	assert.Equal(t, "dog_feed", change.Removed[0])

	_, ok := idx.Task("dog_feed")
	assert.False(t, ok)
	_, ok = idx.Task("other_task")
	assert.True(t, ok, "a deleted file must not affect other files' tasks")
}

// The reconciliation "diff, not rebuild" proof: reconciling one file must
// leave every other file's tasks and diagnostics byte-for-byte untouched.
func TestReconcileFile_DoesNotAffectOtherFiles(t *testing.T) {
	root := seedVault(t, map[string]string{
		"a.md": "- [ ] Task A @id(task_a) @state(active)\n",
		"b.md": "- [ ] Task B @id(task_b) @state(active)\n- [ ] No id here\n",
	})
	idx := New(root, time.UTC)
	require.NoError(t, idx.Build())

	before, ok := idx.Task("task_b")
	require.True(t, ok)
	beforeDiags := idx.Diagnostics()

	require.NoError(t, os.WriteFile(filepath.Join(root, "a.md"), []byte("- [ ] Task A @id(task_a) @state(paused)\n"), 0o644))
	_, err := idx.ReconcileFile("a.md")
	require.NoError(t, err)

	after, ok := idx.Task("task_b")
	require.True(t, ok)
	assert.Equal(t, before, after, "b.md's task must be untouched by reconciling a.md")

	var bDiagsBefore, bDiagsAfter int
	for _, d := range beforeDiags {
		if d.FilePath == "b.md" {
			bDiagsBefore++
		}
	}
	for _, d := range idx.Diagnostics() {
		if d.FilePath == "b.md" {
			bDiagsAfter++
		}
	}
	assert.Equal(t, bDiagsBefore, bDiagsAfter)
}

func TestReconcileFile_WithinFileDuplicateNotBothClaimed(t *testing.T) {
	root := seedVault(t, map[string]string{
		"household.md": "- [ ] First @id(dup)\n- [ ] Second @id(dup)\n",
	})
	idx := New(root, time.UTC)
	require.NoError(t, idx.Build())

	task, ok := idx.Task("dup")
	require.True(t, ok)
	assert.Equal(t, 1, task.Line, "first line in the file wins")

	diags := idx.Diagnostics()
	require.Len(t, diags, 1)
	assert.Equal(t, "duplicate_id", *diags[0].Code)
	assert.Equal(t, 2, diags[0].Line)
}

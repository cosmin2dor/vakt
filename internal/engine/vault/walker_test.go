package vault

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedVault writes files (relative-path -> content) under a fresh temp
// directory and returns the directory's root.
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

func TestWalk_SeededVaultYieldsTasksRetrievableByID(t *testing.T) {
	root := seedVault(t, map[string]string{
		"chores.md": "- [ ] Feed the dog @id(dog_feed) @schedule(0 8 * * *)\n" +
			"- [ ] Just a note, no directive at all\n" +
			"- [ ] Water the plants @id(water_plants)\n",
		"nested/errands.md": "- [ ] Buy milk @id(buy_milk)\n",
	})

	tasks, problems, err := Walk(root)
	require.NoError(t, err)
	assert.Empty(t, problems)

	require.Len(t, tasks, 3)

	dogFeed, ok := tasks["dog_feed"]
	require.True(t, ok, "expected task retrievable by id %q", "dog_feed")
	assert.Equal(t, "chores.md", dogFeed.Path)
	assert.Equal(t, 1, dogFeed.Line)
	assert.Contains(t, dogFeed.Raw, "Feed the dog")

	waterPlants, ok := tasks["water_plants"]
	require.True(t, ok)
	assert.Equal(t, "chores.md", waterPlants.Path)
	assert.Equal(t, 3, waterPlants.Line)

	buyMilk, ok := tasks["buy_milk"]
	require.True(t, ok)
	assert.Equal(t, "nested/errands.md", buyMilk.Path)
	assert.Equal(t, 1, buyMilk.Line)
}

func TestWalk_LineWithoutIDIsNotATaskAndNotAProblem(t *testing.T) {
	// SDD.md G5: a task line without @id does not schedule — and at
	// this layer, doesn't even raise a diagnostic. It's simply not a
	// task line.
	root := seedVault(t, map[string]string{
		"notes.md": "- [ ] Nothing to see here\nJust prose.\n",
	})

	tasks, problems, err := Walk(root)
	require.NoError(t, err)
	assert.Empty(t, tasks)
	assert.Empty(t, problems)
}

func TestWalk_InvalidIDIsReportedAndNotATask(t *testing.T) {
	// SDD.md G3: @id must match [a-z0-9_-]{1,64}.
	root := seedVault(t, map[string]string{
		"chores.md": "- [ ] Uppercase not allowed @id(Dog_Feed)\n" +
			"- [ ] Spaces not allowed @id(dog feed)\n" +
			"- [ ] Empty not allowed @id()\n" +
			"- [ ] Fine @id(ok_id)\n",
	})

	tasks, problems, err := Walk(root)
	require.NoError(t, err)

	require.Len(t, tasks, 1)
	_, ok := tasks["ok_id"]
	assert.True(t, ok)

	require.Len(t, problems, 3)
	for _, p := range problems {
		assert.Equal(t, InvalidID, p.Kind)
		assert.Equal(t, "chores.md", p.Path)
	}
	assert.Equal(t, "Dog_Feed", problems[0].ID)
	assert.Equal(t, 1, problems[0].Line)
	assert.Equal(t, "dog feed", problems[1].ID)
	assert.Equal(t, 2, problems[1].Line)
	assert.Equal(t, "", problems[2].ID)
	assert.Equal(t, 3, problems[2].Line)
}

func TestWalk_DuplicateIDAcrossFilesFirstWinsByPathThenLine(t *testing.T) {
	// SDD.md G4: first wins by path then line — "a-chores.md" sorts
	// before "b-chores.md".
	root := seedVault(t, map[string]string{
		"a-chores.md": "- [ ] First claim @id(shared_id)\n",
		"b-chores.md": "- [ ] Second claim, should lose @id(shared_id)\n",
	})

	tasks, problems, err := Walk(root)
	require.NoError(t, err)

	require.Len(t, tasks, 1)
	winner, ok := tasks["shared_id"]
	require.True(t, ok)
	assert.Equal(t, "a-chores.md", winner.Path)
	assert.Contains(t, winner.Raw, "First claim")

	require.Len(t, problems, 1)
	assert.Equal(t, DuplicateID, problems[0].Kind)
	assert.Equal(t, "b-chores.md", problems[0].Path)
	assert.Equal(t, "shared_id", problems[0].ID)
	assert.Equal(t, 1, problems[0].Line)
}

func TestWalk_DuplicateIDWithinSameFileFirstWinsByLineNumber(t *testing.T) {
	root := seedVault(t, map[string]string{
		"chores.md": "- [ ] First @id(dup)\n- [ ] Second, should lose @id(dup)\n",
	})

	tasks, problems, err := Walk(root)
	require.NoError(t, err)

	require.Len(t, tasks, 1)
	assert.Equal(t, 1, tasks["dup"].Line)
	assert.Contains(t, tasks["dup"].Raw, "First")

	require.Len(t, problems, 1)
	assert.Equal(t, DuplicateID, problems[0].Kind)
	assert.Equal(t, 2, problems[0].Line)
}

func TestWalk_NonMarkdownFilesAreIgnored(t *testing.T) {
	root := seedVault(t, map[string]string{
		"chores.md":  "- [ ] Real task @id(real_task)\n",
		"README.txt": "- [ ] Not markdown @id(should_not_appear)\n",
		"notes.MD":   "- [ ] Uppercase extension still counts @id(uppercase_ext)\n",
	})

	tasks, problems, err := Walk(root)
	require.NoError(t, err)
	assert.Empty(t, problems)

	_, ok := tasks["real_task"]
	assert.True(t, ok)
	_, ok = tasks["should_not_appear"]
	assert.False(t, ok)
	_, ok = tasks["uppercase_ext"]
	assert.True(t, ok, "case-insensitive .md extension should still be discovered")
}

func TestWalk_VaultRelativePaths(t *testing.T) {
	// Paths in Task must be vault-relative, not absolute — a vault is
	// meant to be portable across machines.
	root := seedVault(t, map[string]string{
		"deeply/nested/dir/chores.md": "- [ ] Task @id(nested_task)\n",
	})

	tasks, _, err := Walk(root)
	require.NoError(t, err)

	task, ok := tasks["nested_task"]
	require.True(t, ok)
	assert.Equal(t, "deeply/nested/dir/chores.md", task.Path)
	assert.False(t, filepath.IsAbs(task.Path))
}

func TestWalk_NonexistentRootIsAnError(t *testing.T) {
	_, _, err := Walk(filepath.Join(t.TempDir(), "does-not-exist"))
	require.Error(t, err)
}

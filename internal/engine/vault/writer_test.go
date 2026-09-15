package vault

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWriteSerializesSameFile drives concurrent writers at one path and
// checks two things: the writeLockedHook seam never observes two
// goroutines inside the critical section at once (real serialization, not
// just a lucky outcome of independent renames), and the final file holds
// exactly one submitted content in full.
func TestWriteSerializesSameFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task.md")
	if err := os.WriteFile(path, []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}

	var inCriticalSection int32
	var overlaps int32
	writeLockedHook = func() {
		if atomic.AddInt32(&inCriticalSection, 1) > 1 {
			atomic.AddInt32(&overlaps, 1)
		}
		time.Sleep(2 * time.Millisecond)
		atomic.AddInt32(&inCriticalSection, -1)
	}
	defer func() { writeLockedHook = nil }()

	w := NewWriter(nil)

	const n = 20
	contents := make([][]byte, n)
	for i := range contents {
		contents[i] = bytes.Repeat([]byte{byte('a' + i)}, 4096)
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(content []byte) {
			defer wg.Done()
			if _, err := w.Write(path, content); err != nil {
				t.Errorf("write: %v", err)
			}
		}(contents[i])
	}
	wg.Wait()

	if overlaps > 0 {
		t.Fatalf("critical section overlapped %d times: writes did not serialize", overlaps)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	matched := false
	for _, c := range contents {
		if bytes.Equal(got, c) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("final file did not match any single submitted write in full (len=%d)", len(got))
	}
}

// TestWriteDifferentPathsDoNotBlock asserts writes to distinct paths run
// concurrently rather than serializing against each other: N writers each
// delaying before their write should finish in about one delay's worth of
// wall time, not N times that.
func TestWriteDifferentPathsDoNotBlock(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(nil)

	const n = 8
	paths := make([]string, n)
	for i := range paths {
		p := filepath.Join(dir, fmt.Sprintf("file%d.md", i))
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths[i] = p
	}

	start := time.Now()
	var wg sync.WaitGroup
	for _, p := range paths {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			time.Sleep(20 * time.Millisecond)
			if _, err := w.Write(p, []byte("written")); err != nil {
				t.Errorf("write %s: %v", p, err)
			}
		}(p)
	}
	wg.Wait()
	elapsed := time.Since(start)

	if elapsed > 150*time.Millisecond {
		t.Fatalf("writes to distinct paths appear serialized: took %v for %d writers", elapsed, n)
	}
}

// TestReaderNeverObservesPartialFile forces a large write to race concurrent
// reads and asserts every read is the complete old content or the complete
// new content, never a truncated or mixed one.
func TestReaderNeverObservesPartialFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.md")

	oldContent := bytes.Repeat([]byte("O"), 8*1024*1024)
	if err := os.WriteFile(path, oldContent, 0o644); err != nil {
		t.Fatal(err)
	}
	newContent := bytes.Repeat([]byte("N"), 8*1024*1024)

	w := NewWriter(nil)

	stop := make(chan struct{})
	var badRead atomic.Value

	var readers sync.WaitGroup
	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				got, err := os.ReadFile(path)
				if err != nil {
					badRead.Store(err.Error())
					return
				}
				if !bytes.Equal(got, oldContent) && !bytes.Equal(got, newContent) {
					badRead.Store(fmt.Sprintf("observed partial/mixed content of length %d", len(got)))
					return
				}
			}
		}()
	}

	if _, err := w.Write(path, newContent); err != nil {
		t.Fatal(err)
	}
	close(stop)
	readers.Wait()

	if v := badRead.Load(); v != nil {
		t.Fatalf("reader observed bad content: %v", v)
	}
}

// TestHashRegistryQueryable confirms Record/Check round-trip a hash,
// reflect an update to different content, and that a match is consumed.
func TestHashRegistryQueryable(t *testing.T) {
	reg := NewHashRegistry()
	path := "notes/todo.md"

	h1 := sha256.Sum256([]byte("first version"))
	if reg.Check(path, h1) {
		t.Fatal("expected no match before Record")
	}

	reg.Record(path, h1)
	if !reg.Check(path, h1) {
		t.Fatal("expected match after Record")
	}
	if reg.Check(path, h1) {
		t.Fatal("expected Check to consume the match")
	}

	h2 := sha256.Sum256([]byte("second version"))
	reg.Record(path, h2)
	if reg.Check(path, h1) {
		t.Fatal("stale hash must not match after an update")
	}
	if !reg.Check(path, h2) {
		t.Fatal("expected match for updated hash")
	}
}

// TestWriterRecordsHashOnWrite confirms Write populates a configured
// HashRegistry with the hash of the bytes it published.
func TestWriterRecordsHashOnWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task.md")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := NewHashRegistry()
	w := NewWriter(reg)

	content := []byte("@id(laundry) @cron(0 9 * * 1)")
	hash, err := w.Write(path, content)
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(content)
	if hash != want {
		t.Fatal("returned hash mismatch")
	}
	if !reg.Check(path, want) {
		t.Fatal("expected registry to hold the written content's hash")
	}
}

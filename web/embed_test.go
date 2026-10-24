package web

import (
	"io/fs"
	"testing"
)

func TestDistFS(t *testing.T) {
	dfs := AssetsFS()
	f, err := dfs.Open("/")
	if err != nil {
		t.Fatalf("failed to open assets root: %v", err)
	}
	defer f.Close()

	if dir, ok := f.(fs.ReadDirFile); ok {
		entries, err := dir.ReadDir(-1)
		if err != nil || len(entries) == 0 {
			t.Fatalf("expected assets entries, got err=%v, count=%d", err, len(entries))
		}
	}
}

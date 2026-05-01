package imagegen

import (
	"path/filepath"
	"testing"
)

func TestModelCacheDir(t *testing.T) {
	t.Parallel()

	got := ModelCacheDir("data/images", "gpt-image-2")
	want := filepath.Join("data/images", "gpt-image-2")
	if got != want {
		t.Fatalf("ModelCacheDir() = %q, want %q", got, want)
	}
}

func TestModelCacheDir_UsesDefaultModelWhenEmpty(t *testing.T) {
	t.Parallel()

	got := ModelCacheDir("data/images", "")
	want := filepath.Join("data/images", DefaultModel)
	if got != want {
		t.Fatalf("ModelCacheDir() = %q, want %q", got, want)
	}
}

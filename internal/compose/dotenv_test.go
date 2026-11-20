package compose

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseDotenv(t *testing.T) {
	in := "\xef\xbb\xbf# comment\n\nA=1\r\nexport B=two words\nC=\"quoted # not comment\"\nD='single'\nE=value # trailing\nF=\nnot a pair\n1BAD=x\n  G = spaced  \n"
	want := map[string]string{"A": "1", "B": "two words", "C": "quoted # not comment", "D": "single", "E": "value", "F": "", "G": "spaced"}
	if got := ParseDotenv([]byte(in)); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestReadRegularFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.env")
	if err := os.WriteFile(p, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readRegularFile(p, 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := readRegularFile(p, 2); err == nil || !strings.Contains(err.Error(), "larger") {
		t.Fatalf("expected size error, got %v", err)
	}
	if _, err := readRegularFile(dir, 10); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("expected ErrNotRegular, got %v", err)
	}
	if _, err := readRegularFile(filepath.Join(dir, "missing"), 10); !os.IsNotExist(err) {
		t.Fatalf("expected not-exist, got %v", err)
	}
}

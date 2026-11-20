package compose

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// MaxAuxFileSize bounds the size of auxiliary files (.env, env_file) we read.
const MaxAuxFileSize = 1 << 20

// ErrNotRegular is returned when a path exists but is not a regular file.
var ErrNotRegular = errors.New("not a regular file")

// ParseDotenv parses KEY=VALUE lines as used by .env and env_file files.
// Supported: blank lines, "#" comments, an optional "export " prefix,
// single- and double-quoted values and inline " #" comments on unquoted
// values. Lines without "=" are ignored.
func ParseDotenv(data []byte) map[string]string {
	out := map[string]string{}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), MaxAuxFileSize)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimSuffix(sc.Text(), "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if !validName(key) {
			continue
		}
		out[key] = unquote(strings.TrimSpace(val))
	}
	return out
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') {
		q := v[0]
		if end := strings.IndexByte(v[1:], q); end >= 0 {
			return v[1 : end+1]
		}
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v
}

func validName(s string) bool {
	if s == "" || !isNameStart(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isNameChar(s[i]) && s[i] != '.' && s[i] != '-' {
			return false
		}
	}
	return true
}

// readRegularFile reads at most limit bytes from a regular file. Symlinks
// are followed (Docker follows them too) but devices, FIFOs and directories
// are refused so a hostile project cannot make the tool block or read
// unbounded data.
func readRegularFile(path string, limit int64) ([]byte, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, ErrNotRegular
	}
	if fi.Size() > limit {
		return nil, fmt.Errorf("file is larger than %d bytes", limit)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file is larger than %d bytes", limit)
	}
	return data, nil
}

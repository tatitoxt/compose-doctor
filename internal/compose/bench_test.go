package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkLoad measures end-to-end project loading (discovery, YAML parse,
// interpolation and multi-file merge) on the fixture corpus.
func BenchmarkLoad(b *testing.B) {
	fixtures := map[string]Options{
		"valid-minimal": {Path: "../../testdata/fixtures/valid-minimal"},
		"valid-complex": {Path: "../../testdata/fixtures/valid-complex"},
		"profiles":      {Path: "../../testdata/fixtures/profiles"},
		"multi-file": {
			Path:  "../../testdata/fixtures/multi-file",
			Files: []string{"base.yaml", "prod.yaml"},
		},
	}
	for name, opts := range fixtures {
		b.Run(name, func(b *testing.B) {
			opts.Environ = []string{}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := Load(opts); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkLoadManyServices measures parse+merge throughput on a generated
// project with 200 services, approximating a large real-world Compose file.
func BenchmarkLoadManyServices(b *testing.B) {
	dir := b.TempDir()
	content := "services:\n"
	for i := 0; i < 200; i++ {
		content += fmt.Sprintf(`  svc-%03d:
    image: registry.example.com/app:%d
    environment:
      KEY: value-%d
    ports:
      - "%d:80"
`, i, i, i, 8000+i)
	}
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(content), 0o644); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Load(Options{Path: dir, Environ: []string{}}); err != nil {
			b.Fatal(err)
		}
	}
}

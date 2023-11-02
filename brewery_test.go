package brewery

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"

	"github.com/maxmcd/brewery/tracing"
	"github.com/maxmcd/reptar"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sync/errgroup"
	"gopkg.in/dnaeon/go-vcr.v3/recorder"
)

func TestFormulaHack(t *testing.T) {
	formulaIndex := map[string][2]int{}

	searchPrefix := []byte("\n  {\"name\":\"")
	newlineAndSpaceLen := len([]byte("\n  "))
	commaLen := len([]byte(","))

	f, err := os.Open("/Users/maxm/.cache/brewery/api/formula.json")
	if err != nil {
		t.Fatal(err)
	}

	r := bufio.NewReader(f)

	{
		// open bracket
		bs, err := r.ReadBytes('\n')
		fmt.Println(string(bs), err)
	}
	{
		bs, err := r.ReadBytes('\n')
		fmt.Println(string(bs), err)
		namePrefix := []byte(`name":"`)
		fmt.Println(string(bs[bytes.Index(bs, namePrefix)+len(namePrefix) : bytes.Index(bs, []byte(`",`))]))
		var formula Formula
		fmt.Println(json.Unmarshal(bs[:bytes.LastIndexByte(bs, '}')+1], &formula))
		fmt.Println(formula)
	}
	return

	f.Seek(24698064, 0)
	b := make([]byte, 4015)
	_, _ = f.Read(b)
	fmt.Println(string(b))
	return

	scanner := bufio.NewScanner(f)

	position := 0
	count := 0
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		start := bytes.Index(data, searchPrefix)
		if start == -1 {
			return 0, nil, nil
		}
		end := bytes.Index(data[start+len(searchPrefix):], searchPrefix)
		if end == -1 {
			if !atEOF {
				return 0, nil, nil
			}
			end = bytes.LastIndex(data, []byte("}")) + 1 - len(searchPrefix)
		}

		offset := start + newlineAndSpaceLen
		size := end + len(searchPrefix) - commaLen - newlineAndSpaceLen
		fmt.Println(position+offset, size)
		advance = offset + size
		position += advance
		count += 1
		fmt.Println(advance)
		if atEOF {
			// Scanner will not scan the final bytes after EOF unless we return
			// bytes. Return bytes here so that the input advances.
			// Alternatively we could loop over the buffer ourselves and process
			// each section.
			return advance, data[offset : offset+size], nil
		}
		return advance, nil, nil
	})

	for scanner.Scan() {
		_ = scanner.Text()
	}

	fmt.Println(count)
	_ = f
	_ = formulaIndex
}

func Test_cloneDirWithSymlinks(t *testing.T) {
	if err := cloneDirWithSymlinks("/home/linuxbrew/.linuxbrew/Cellar/ruby", t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkSym(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if err := cloneDirWithSymlinks("/home/linuxbrew/.linuxbrew/Cellar/ruby", b.TempDir()); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGzipUnarchive(b *testing.B) {
	for i := 0; i < b.N; i++ {
		f, err := os.Open("/home/ubuntu/.cache/Homebrew/downloads/843ec2129e032ac407cc17cf9141a6ce69f8f0556061f6e1de7ecee17f4ae971--ruby--3.2.2.x86_64_linux.bottle.tar.gz")
		if err != nil {
			b.Fatal(err)
		}
		if err := reptar.GzipUnarchive(f, b.TempDir()); err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark4GzipUnarchive(b *testing.B) {
	for i := 0; i < b.N; i++ {
		eg, _ := errgroup.WithContext(context.Background())
		for i := 0; i < 8; i++ {
			eg.Go(func() error {
				f, err := os.Open("/home/ubuntu/.cache/Homebrew/downloads/843ec2129e032ac407cc17cf9141a6ce69f8f0556061f6e1de7ecee17f4ae971--ruby--3.2.2.x86_64_linux.bottle.tar.gz")
				if err != nil {
					return err
				}
				if err := reptar.GzipUnarchive(f, b.TempDir()); err != nil {
					return err
				}
				return nil
			})
		}
		if err := eg.Wait(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTarGzipUnarchive(b *testing.B) {
	for i := 0; i < b.N; i++ {
		f, err := os.Open("/home/ubuntu/.cache/Homebrew/downloads/843ec2129e032ac407cc17cf9141a6ce69f8f0556061f6e1de7ecee17f4ae971--ruby--3.2.2.x86_64_linux.bottle.tar.gz")
		if err != nil {
			b.Fatal(err)
		}
		cmd := exec.Command("tar", "-z", "--extract", "--no-same-owner", "--directory", b.TempDir())
		cmd.Stdin = f

		s, err := cmd.CombinedOutput()
		if err != nil {
			b.Fatal(fmt.Errorf("%s: %w", string(s), err))
		}
	}
}

type T interface {
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	TempDir() string
	Cleanup(func())
}

func newRecorder(t T) *recorder.Recorder {
	recorder, err := recorder.NewWithOptions(&recorder.Options{
		CassetteName: "brewery-recorder",
		// Mode:         recorder.ModeReplayWithNewEpisodes,
		// Mode:         recorder.ModeRecordOnly,
		Mode:               recorder.ModeReplayOnly,
		SkipRequestLatency: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := recorder.Stop(); err != nil {
			t.Fatal(err)
		}
	})
	return recorder
}

func brewery(t T) *Brewery {
	recorder := newRecorder(t)
	b, err := NewBrewery(
		OptionWithHTTPClient(&http.Client{Transport: recorder}),
		OptionWithCache(t.TempDir()),
	)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestInstall(t *testing.T) {
	names := []string{"Install", "InstallParallel", "InstallParallel2"}
	for i, fn := range []func(context.Context, *Brewery) error{
		// func(ctx context.Context, b *Brewery) error {
		// 	return b.Install(ctx, "ruby")
		// },
		// func(ctx context.Context, b *Brewery) error {
		// 	return b.InstallParallel(ctx, "ruby")
		// },
		func(ctx context.Context, b *Brewery) error {
			return b.InstallParallel2(ctx, "ruby")
		},
	} {
		t.Run(names[i], func(t *testing.T) {
			ctx, span := networkTracer.Start(context.Background(), names[i])
			defer span.End()
			br := brewery(t)
			if err := fn(ctx, br); err != nil {
				t.Fatal(err)
			}
		})
	}
	tracing.Stop()
}

func TestFormulaIndex(t *testing.T) {
	b, err := NewBrewery()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(b.cache("api", "formula.json"))
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(f)
	scanner.Split(jsonObjectSplitFunc)

	for scanner.Scan() {
		foo := scanner.Text()
		_ = foo
	}
}

func BenchmarkFormulaIndex(b *testing.B) {
	br, err := NewBrewery()
	if err != nil {
		b.Fatal(err)
	}
	f, err := os.Open(br.cache("api", "formula.json"))
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < b.N; i++ {
		_, _ = f.Seek(0, 0)
		scanner := bufio.NewScanner(f)
		scanner.Split(jsonObjectSplitFunc)

		for scanner.Scan() {
			foo := scanner.Text()
			_ = foo
		}
	}
}

func BenchmarkFormulaNoIndex(b *testing.B) {
	br, err := NewBrewery()
	if err != nil {
		b.Fatal(err)
	}
	f, err := os.Open(br.cache("api", "formula.json"))
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < b.N; i++ {
		_, _ = f.Seek(0, 0)
		_, _ = findFormulas(context.Background(), f, "ruby")
	}
}

func Test_jsonObjectSplitFunc(t *testing.T) {
	for _, tt := range []struct {
		src   string
		lines []string
	}{
		{"[{},{{}}]", []string{"{}", "{{}}"}},
		{`[{},{"f":"\}"}]`, []string{"{}", `{"f":"\}"}`}},
		{"[ {},    {{}}, ]", []string{"{}", "{{}}"}},
		{"{},{{}}", []string{"{}", "{{}}"}},
		{"[{},{{", []string{"{}"}},
	} {
		t.Run(tt.src, func(t *testing.T) {
			scanner := bufio.NewScanner(bytes.NewBufferString(tt.src))
			scanner.Split(jsonObjectSplitFunc)

			out := []string{}
			for scanner.Scan() {
				out = append(out, scanner.Text())
			}
			assert.Equal(t, tt.lines, out)
		})
	}
}

func BenchmarkInstall(b *testing.B) {
	for i := 0; i < b.N; i++ {
		br := brewery(b)
		if err := br.Install(context.Background(), "ruby"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkInstallParallel(b *testing.B) {
	for i := 0; i < b.N; i++ {
		br := brewery(b)
		if err := br.InstallParallel(context.Background(), "ruby"); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkInstallParallel2(b *testing.B) {
	for i := 0; i < b.N; i++ {
		br := brewery(b)
		if err := br.InstallParallel2(context.Background(), "ruby"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkManifestCalls(b *testing.B) {
	ctx := context.Background()
	br, err := NewBrewery()
	if err != nil {
		b.Fatal(err)
	}

	f, err := os.Open(br.cache("api", "formula.json"))
	if err != nil {
		b.Fatal(err)
	}
	formulas, err := findFormulas(ctx, f, "ruby")
	if err != nil {
		b.Fatal(err)
	}
	formula := formulas[0]

	m, err := br.DownloadManifest(ctx, formula)
	if err != nil {
		b.Fatal(err)
	}

	tb, err := m.TabForCurrentOS()
	if err != nil {
		b.Fatal(err)
	}

	_, _ = f.Seek(0, 0)
	formulas, err = findFormulas(ctx, f, mapSlice(tb.RuntimeDependencies, func(d Dependency) string {
		return d.FullName
	})...)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eg, ctx := errgroup.WithContext(context.Background())
		for _, f := range formulas {
			f := f
			eg.Go(func() error {
				m, err := br.DownloadManifest(ctx, f)
				_ = m
				return err
			})
		}
		if err := eg.Wait(); err != nil {
			b.Fatal(err)
		}
	}
}

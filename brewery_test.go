package brewery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/maxmcd/brewery/tracing"
	"github.com/maxmcd/reptar"
	"golang.org/x/sync/errgroup"
)

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

func TestFormula(t *testing.T) {
	ctx, span := networkTracer.Start(context.Background(), "brewery-test")
	defer span.End()
	defer tracing.Stop()
	f, err := os.Open("/home/ubuntu/.cache/Homebrew/api/formula.json")
	if err != nil {
		t.Fatal(err)
	}
	formulas, err := findFormulas(f, "ruby")
	if err != nil {
		t.Fatal(err)
	}
	formula := formulas[0]

	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	_ = e.Encode(formula)

	b, err := NewBrewery()
	if err != nil {
		t.Fatal(err)
	}

	m, err := b.FetchManifest(ctx, formula.ManifestURL())
	if err != nil {
		t.Fatal(err)
	}

	tb, err := m.TabForCurrentOS()
	if err != nil {
		t.Fatal(err)
	}
	{
		e := json.NewEncoder(os.Stdout)
		e.SetIndent("", "  ")
		_ = e.Encode(tb)
	}

	_, _ = f.Seek(0, 0)
	formulas, err = findFormulas(f, mapSlice(tb.RuntimeDependencies, func(d Dependency) string {
		return d.FullName
	})...)
	if err != nil {
		t.Fatal(err)
	}
	{
		e := json.NewEncoder(os.Stdout)
		e.SetIndent("", "  ")
		_ = e.Encode(mapSlice(formulas, func(f Formula) string { return f.Name }))
	}
	dir := t.TempDir()

	eg, ctx := errgroup.WithContext(ctx)
	for _, f := range formulas {
		formula := f
		eg.Go(func() error {
			url, err := b.StableBottleURL(formula)
			if err != nil {
				return err
			}
			start := time.Now()
			resp, err := b.FetchBottleFiles(ctx, url)
			if err != nil {
				return err
			}
			_ = dir
			f, err := os.CreateTemp(dir, "")
			if err != nil {
				return err
			}
			_, _ = io.Copy(f, resp.Body)
			_ = f.Close()
			_ = resp.Body.Close()
			fmt.Printf("%s %s: %s %s\n", url, resp.Header.Get("Content-Length"), formula.Name, time.Since(start))
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}
}

type T interface {
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
}

func BenchmarkManifestCalls(b *testing.B) {
	f, err := os.Open("/home/ubuntu/.cache/Homebrew/api/formula.json")
	if err != nil {
		b.Fatal(err)
	}
	formulas, err := findFormulas(f, "ruby")
	if err != nil {
		b.Fatal(err)
	}
	formula := formulas[0]

	br, err := NewBrewery()
	if err != nil {
		b.Fatal(err)
	}

	m, err := br.FetchManifest(context.Background(), formula.ManifestURL())
	if err != nil {
		b.Fatal(err)
	}

	tb, err := m.TabForCurrentOS()
	if err != nil {
		b.Fatal(err)
	}

	_, _ = f.Seek(0, 0)
	formulas, err = findFormulas(f, mapSlice(tb.RuntimeDependencies, func(d Dependency) string {
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
				m, err := br.FetchManifest(ctx, f.ManifestURL())
				_ = m
				return err
			})
		}
		if err := eg.Wait(); err != nil {
			b.Fatal(err)
		}
	}
}

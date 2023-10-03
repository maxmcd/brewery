package brewery

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/maxmcd/reptar"
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

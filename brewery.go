package brewery

import (
	"fmt"
	"os"
	"path/filepath"
)

type Brewery struct {
	prefix string
}

func NewBrewery() (*Brewery, error) {
	prefix, err := getBrewPrefix()
	if err != nil {
		return nil, fmt.Errorf("error getting brew prefix: %w: %q", err, prefix)
	}
	return &Brewery{prefix: prefix}, nil
}
func (b *Brewery) cellar(a ...string) string {
	return filepath.Join(append([]string{b.prefix, "/Cellar"}, a...)...)
}

func (b *Brewery) getVersion(name string) {
	os.Stat(b.cellar(name))
}

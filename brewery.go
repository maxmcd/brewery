package brewery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

var (
	brewAPIRoot = "https://formulae.brew.sh/api/"
)

type Brewery struct {
	prefix string

	httpClient *http.Client
}

type Option func(b *Brewery)

func OptionWithHTTPClient(httpCLient *http.Client) func(*Brewery) {
	return func(b *Brewery) { b.httpClient = httpCLient }
}

func NewBrewery(opts ...Option) (*Brewery, error) {
	prefix, err := getBrewPrefix()
	if err != nil {
		return nil, fmt.Errorf("error getting brew prefix: %w: %q", err, prefix)
	}
	b := &Brewery{prefix: prefix}
	for _, o := range opts {
		o(b)
	}
	return b, nil
}
func (b *Brewery) cellar(a ...string) string {
	return filepath.Join(append([]string{b.prefix, "/Cellar"}, a...)...)
}

func (b *Brewery) getLocalVersion(name string) {
	os.Stat(b.cellar(name))
	// TODO...
}

func (b *Brewery) FetchFormula(name string) (Formula, error) {
	url := brewAPIRoot + "formula/" + name + ".json"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return Formula{}, err
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return Formula{}, fmt.Errorf("error making %s request to %s: %w", http.MethodGet, url, err)
	}
	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		if resp.Body != nil {
			_, _ = io.Copy(&buf, resp.Body)
		}
		return Formula{}, fmt.Errorf("unexpected status code %d when making %s request to %s: %s",
			resp.StatusCode, http.MethodGet, url, buf.String())
	}
	var f Formula
	if err := json.NewDecoder(resp.Body).Decode(&f); err != nil {
		return Formula{}, fmt.Errorf("error parsing json for formula %q: %w", name, err)
	}
	return f, nil
}

func (f Formula) ManifestURL() string {
	u := fmt.Sprintf(
		"%s/%s/manifests/%s",
		f.Bottle.Stable.RootURL,
		f.Name, f.Versions.Stable,
	)
	if f.Revision != 0 {
		u += fmt.Sprintf("_%d", f.Revision)
	}
	return u
}

type Formula struct {
	Name              string   `json:"name"`
	FullName          string   `json:"full_name"`
	Tap               string   `json:"tap"`
	Oldname           string   `json:"oldname"`
	Oldnames          []string `json:"oldnames"`
	Aliases           []string `json:"aliases"`
	VersionedFormulae []string `json:"versioned_formulae"`
	Desc              string   `json:"desc"`
	License           string   `json:"license"`
	Homepage          string   `json:"homepage"`
	Versions          struct {
		Stable string `json:"stable"`
		Head   string `json:"head"`
		Bottle bool   `json:"bottle"`
	} `json:"versions"`
	Urls struct {
		Stable struct {
			URL      string `json:"url"`
			Tag      string `json:"tag"`
			Revision string `json:"revision"`
			Checksum string `json:"checksum"`
		} `json:"stable"`
		Head struct {
			URL    string `json:"url"`
			Branch string `json:"branch"`
		} `json:"head"`
	} `json:"urls"`
	Revision      int `json:"revision"`
	VersionScheme int `json:"version_scheme"`
	Bottle        struct {
		Stable struct {
			Rebuild int    `json:"rebuild"`
			RootURL string `json:"root_url"`
			Files   map[string]struct {
				Cellar string `json:"cellar"`
				URL    string `json:"url"`
				Sha256 string `json:"sha256"`
			} `json:"files"`
		} `json:"stable"`
	} `json:"bottle"`
	KegOnly       bool `json:"keg_only"`
	KegOnlyReason *struct {
		Reason      string
		explanation string
	} `json:"keg_only_reason"`
	Options                 []string      `json:"options"`
	BuildDependencies       []string      `json:"build_dependencies"`
	Dependencies            []string      `json:"dependencies"`
	TestDependencies        []string      `json:"test_dependencies"`
	RecommendedDependencies []string      `json:"recommended_dependencies"`
	OptionalDependencies    []string      `json:"optional_dependencies"`
	UsesFromMacos           []interface{} `json:"uses_from_macos"`
	UsesFromMacosBounds     []interface{} `json:"uses_from_macos_bounds"`
	Requirements            []struct {
		Name     string   `json:"name"`
		Cask     string   `json:"cask"`
		Download string   `json:"download"`
		Version  string   `json:"version"`
		Contexts []string `json:"contexts"`
		Specs    []string `json:"specs"`
	} `json:"requirements"`
	ConflictsWith        []string `json:"conflicts_with"`
	ConflictsWithReasons []string `json:"conflicts_with_reasons"`
	LinkOverwrite        []string `json:"link_overwrite"`
	Caveats              string   `json:"caveats"`
	Installed            []struct {
		Version             string   `json:"version"`
		UsedOptions         []string `json:"used_options"`
		BuiltAsBottle       bool     `json:"built_as_bottle"`
		PouredFromBottle    bool     `json:"poured_from_bottle"`
		Time                int      `json:"time"`
		RuntimeDependencies []struct {
			FullName         string `json:"full_name"`
			Version          string `json:"version"`
			DeclaredDirectly bool   `json:"declared_directly"`
		} `json:"runtime_dependencies"`
		InstalledAsDependency bool `json:"installed_as_dependency"`
		InstalledOnRequest    bool `json:"installed_on_request"`
	} `json:"installed"`
	LinkedKeg          string      `json:"linked_keg"`
	Pinned             bool        `json:"pinned"`
	Outdated           bool        `json:"outdated"`
	Deprecated         bool        `json:"deprecated"`
	DeprecationDate    string      `json:"deprecation_date"`
	DeprecationReason  string      `json:"deprecation_reason"`
	Disabled           bool        `json:"disabled"`
	DisableDate        string      `json:"disable_date"`
	DisableReason      string      `json:"disable_reason"`
	PostInstallDefined bool        `json:"post_install_defined"`
	Service            interface{} `json:"service"`
	TapGitHead         string      `json:"tap_git_head"`
	RubySourcePath     string      `json:"ruby_source_path"`
	RubySourceChecksum struct {
		Sha256 string `json:"sha256"`
	} `json:"ruby_source_checksum"`
	Variations map[string]struct {
		BuildDependencies []string `json:"build_dependencies"`
		Dependencies      []string `json:"dependencies"`
	} `json:"variations"`
}

type Manifest struct {
	SchemaVersion int `json:"schemaVersion"`
	Manifests     []struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int    `json:"size"`
		Platform  struct {
			Architecture string `json:"architecture"`
			Os           string `json:"os"`
			OsVersion    string `json:"os.version"`
		} `json:"platform"`
		Annotations struct {
			OrgOpencontainersImageRefName string       `json:"org.opencontainers.image.ref.name"`
			ShBrewBottleCPUVariant        string       `json:"sh.brew.bottle.cpu.variant"`
			ShBrewBottleDigest            string       `json:"sh.brew.bottle.digest"`
			ShBrewBottleGlibcVersion      string       `json:"sh.brew.bottle.glibc.version"`
			ShBrewBottleSize              string       `json:"sh.brew.bottle.size"`
			ShBrewTab                     BrewTabField `json:"sh.brew.tab"`
		} `json:"annotations,omitempty"`
	} `json:"manifests"`
	Annotations map[string]string `json:"annotations"`
}

type BrewTab struct {
	HomebrewVersion     string   `json:"homebrew_version"`
	ChangedFiles        []string `json:"changed_files"`
	SourceModifiedTime  int      `json:"source_modified_time"`
	Compiler            string   `json:"compiler"`
	RuntimeDependencies []struct {
		FullName         string `json:"full_name"`
		Version          string `json:"version"`
		DeclaredDirectly bool   `json:"declared_directly"`
	} `json:"runtime_dependencies"`
	Arch    string `json:"arch"`
	BuiltOn struct {
		Os            string `json:"os"`
		OsVersion     string `json:"os_version"`
		CPUFamily     string `json:"cpu_family"`
		Xcode         string `json:"xcode"`
		Clt           string `json:"clt"`
		PreferredPerl string `json:"preferred_perl"`
	} `json:"built_on"`
}

type BrewTabField struct {
	BrewTab
}

var _ json.Unmarshaler = new(BrewTabField)

func (b *BrewTabField) UnmarshalJSON(v []byte) error {
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return err
	}
	var innerB BrewTab
	if err := json.Unmarshal([]byte(s), &innerB); err != nil {
		return err
	}
	*b = BrewTabField{BrewTab: innerB}
	return nil
}

func getBrewPrefix() (string, error) {
	b, err := exec.Command("brew", "--prefix").CombinedOutput()
	if err == nil {
		return string(b), nil
	}
	return "", fmt.Errorf("error calling `brew --prefix`: %w: %s", err, string(b))
}

func getBrewCache() (string, error) {
	b, err := exec.Command("brew", "--cache").CombinedOutput()
	if err == nil {
		return string(b), nil
	}
	return "", fmt.Errorf("error calling `brew --cache`: %w: %s", err, string(b))
}

// cloneDirWithSymlinks clones a directory but writes each file as a symbolic
// link to the source file. This is how brew references file in the
// lib/include/bin directories. Directories are created as needed. src and dst
// must exist, the contents of src are copied into the dst directory.
func cloneDirWithSymlinks(src, dst string) error {
	dst, err := filepath.Abs(dst)
	if err != nil {
		return fmt.Errorf("error finding absolute path for destination folder: %w", err)
	}
	if err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if src == path {
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		dstLocation := filepath.Join(dst, rel)
		if d.IsDir() {
			if err := os.Mkdir(dstLocation, 0777); err != nil && !os.IsExist(err) {
				return fmt.Errorf("error creating dir %q: %w", dstLocation, err)
			}
			return nil
		}
		dir := filepath.Dir(dstLocation)
		srcRelPath, _ := filepath.Rel(dir, path)
		if err := os.Symlink(srcRelPath, dstLocation); err != nil {
			return fmt.Errorf("error symlinking %q to %q: %w", srcRelPath, dstLocation, err)
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

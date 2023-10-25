package brewery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/maxmcd/brewery/tracing"
	"go.opentelemetry.io/otel/attribute"
)

var (
	brewAPIRoot   = "https://formulae.brew.sh/api/"
	networkTracer = tracing.Tracer("network")
	diskTracer    = tracing.Tracer("disk")
)

type Brewery struct {
	prefix        string
	cacheLocation string
	httpClient    *http.Client
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
	cache, err := getBrewCache()
	if err != nil {
		return nil, fmt.Errorf("error getting brew cache: %w: %q", err, cache)
	}

	b := &Brewery{prefix: prefix, cacheLocation: cache}
	for _, o := range opts {
		o(b)
	}
	if b.httpClient == nil {
		b.httpClient = &http.Client{}
		// timeout := 5 * time.Millisecond
		// upto := 2
		// hedged, err := hedgedhttp.NewClient(timeout, upto, &http.Client{})
		// if err != nil {
		// 	return nil, fmt.Errorf("error creating hedged http client: %w", err)
		// }
		// b.httpClient = hedged

	}
	return b, nil
}

func (b *Brewery) cellar(a ...string) string {
	return filepath.Join(append([]string{b.prefix, "/Cellar"}, a...)...)
}

func (b *Brewery) cache(a ...string) string {
	return filepath.Join(append([]string{b.cacheLocation}, a...)...)
}

func (b *Brewery) getLocalVersion(name string) {
	os.Stat(b.cellar(name))
	// TODO...
}

func (b *Brewery) _getRequest(ctx context.Context, url string, rm func(*http.Request)) (resp *http.Response, err error) {
	ctx, span := networkTracer.Start(ctx, "brewery._getRequest")
	span.SetAttributes(attribute.String("url", url))
	defer func() {
		if err != nil {
			span.RecordError(err)
		}
		span.End()
	}()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("error making request for %s: %w", url, err)
	}
	req = req.WithContext(ctx)
	if rm != nil {
		rm(req)
	}
	resp, err = b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making %s request to %s: %w", http.MethodGet, url, err)
	}
	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		if resp.Body != nil {
			_, _ = io.Copy(&buf, resp.Body)
			resp.Body.Close()
		}
		return nil, fmt.Errorf("unexpected status code %d when making %s request to %s: %s",
			resp.StatusCode, http.MethodGet, url, buf.String())
	}
	return resp, nil
}

func (b *Brewery) getRequest(ctx context.Context, url string, rm func(*http.Request), v interface{}) (err error) {
	resp, err := b._getRequest(ctx, url, rm)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("error parsing json for response from %q: %w", url, err)
	}
	return nil
}

func (b *Brewery) FetchFormula(ctx context.Context, name string) (f Formula, err error) {
	url := brewAPIRoot + "formula/" + name + ".json"
	return f, b.getRequest(ctx, url, func(r *http.Request) {}, &f)
}

func (b *Brewery) downloadAllFormulas(ctx context.Context) (err error) {
	u := "https://formulae.brew.sh/api/formula.json"

	resp, err := b._getRequest(ctx, u, nil)
	if err == nil && resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		err = fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	if err != nil {
		return fmt.Errorf("error requesting %q: %w", u, err)
	}
	defer resp.Body.Close()
	loc := b.cache("api", "formula.json")
	f, err := os.Create(loc)
	if err != nil {
		return fmt.Errorf("error opening file %q: %w", loc, err)
	}
	if _, err = io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("error writing to file %q: %w", loc, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("error closing file %q: %w", loc, err)
	}
	return nil
}

func (b *Brewery) Install(ctx context.Context, formula string) (err error) {
	f, err := b.openOrDownloadAllFormulas(ctx)
	if err != nil {
		return fmt.Errorf("error opening or downloading all formulas: %w", err)
	}
	formulas, err := findFormulas(f, formula)
	if err != nil {
		return fmt.Errorf("error finding formula %s: %w", formula, err)
	}
	formulaData := formulas[0]
	m, err := b.FetchManifest(ctx, formulaData.ManifestURL())
	if err != nil {
		return fmt.Errorf("error fetching manifest for %s: %w", formulaData.Name, err)
	}

	tb, err := m.TabForCurrentOS()
	if err != nil {
		return fmt.Errorf("error fetching information about the current os: %w", err)
	}
	dependencyFormulas := mapSlice(tb.RuntimeDependencies, func(d Dependency) string {
		return d.FullName
	})
	_, _ = f.Seek(0, 0)
	formulas, err = findFormulas(f, dependencyFormulas...)
	if err != nil {
		return fmt.Errorf("error finding formulas %v: %w", dependencyFormulas, err)
	}

	return nil
}

func mapSlice[T any, U any](s []T, f func(T) U) []U {
	r := make([]U, len(s))
	for i, v := range s {
		r[i] = f(v)
	}
	return r
}

func (b *Brewery) openOrDownloadAllFormulas(ctx context.Context) (f *os.File, err error) {
	loc := b.cache("api", "formula.json")
	if _, err := os.Stat(loc); err != nil && os.IsNotExist(err) {
		// TODO: handle other errors
		if err := b.downloadAllFormulas(ctx); err != nil {
			return nil, err
		}
	}
	return os.Open(loc)
}

func (b *Brewery) FetchManifest(ctx context.Context, url string) (m Manifest, err error) {
	return m, b.getRequest(ctx, url, prepareGHCRRequest, &m)
}

func (b *Brewery) FetchBottleFiles(ctx context.Context, url string) (resp *http.Response, err error) {
	return b._getRequest(ctx, url, prepareGHCRRequest)
}

func jsonPrettyPrint[T any](t T) {
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	_ = e.Encode(t)
}

func (b *Brewery) StableBottleURL(f Formula) (string, error) {
	files := f.Bottle.Stable.Files[b.bottleOSString()]
	if files.Cellar != ":any" && files.Cellar != ":any_skip_relocation" && files.Cellar != b.cellar() {
		return "", fmt.Errorf("cellar mismatch: %q != %q", files.Cellar, b.prefix)
	}
	return files.URL, nil
}

func (b *Brewery) bottleOSString() string {
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		return "x86_64_linux"
	}
	return ""
}

func prepareGHCRRequest(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json")
	req.Header.Set("Authorization", "Bearer QQ==")
	req.Header.Set("User-Agent", "Brewery/4.1.13 (Linux; x86_64 Ubuntu 22.04.3 LTS) curl/7.81.0")
}

func findFormulas(allFormulas io.Reader, names ...string) (formulas []Formula, err error) {
	nameSet := map[string]struct{}{}
	for _, name := range names {
		nameSet[name] = struct{}{}
	}
	decoder := json.NewDecoder(allFormulas)
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("error decoding first token of formula reader: %w", err)
	}
	for decoder.More() {
		var f Formula
		if err := decoder.Decode(&f); err != nil {
			return nil, fmt.Errorf("error decoding formula within formula list: %w", err)
		}
		if _, found := nameSet[f.Name]; found {
			formulas = append(formulas, f)

			if len(names) == len(formulas) {
				return formulas, nil
			}
		}
	}
	if len(formulas) == len(names) {
		return formulas, nil
	}
	for _, f := range formulas {
		delete(nameSet, f.Name)
	}
	var missing []string
	for name := range nameSet {
		missing = append(missing, name)
	}
	return nil, fmt.Errorf("missing formulas: %v", missing)
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

func (f Formula) ManifestURL() string {
	u := fmt.Sprintf(
		"%s/%s/manifests/%s",
		f.Bottle.Stable.RootURL,
		strings.Replace(f.Name, "@", "/", 1), f.Versions.Stable,
	)
	if f.Revision != 0 {
		u += fmt.Sprintf("_%d", f.Revision)
	}
	if f.Bottle.Stable.Rebuild != 0 {
		u += fmt.Sprintf("-%d", f.Bottle.Stable.Rebuild)
	}
	return u
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

func (m Manifest) TabForCurrentOS() (BrewTab, error) {
	for _, m := range m.Manifests {
		if m.Platform.Os == runtime.GOOS && m.Platform.Architecture == runtime.GOARCH {
			return m.Annotations.ShBrewTab.BrewTab, nil
		}
	}
	return BrewTab{}, fmt.Errorf("no tab found for %s/%s", runtime.GOOS, runtime.GOARCH)
}

type Dependency struct {
	FullName         string `json:"full_name"`
	Version          string `json:"version"`
	DeclaredDirectly bool   `json:"declared_directly"`
}

type BrewTab struct {
	HomebrewVersion     string       `json:"homebrew_version"`
	ChangedFiles        []string     `json:"changed_files"`
	SourceModifiedTime  int          `json:"source_modified_time"`
	Compiler            string       `json:"compiler"`
	RuntimeDependencies []Dependency `json:"runtime_dependencies"`
	Arch                string       `json:"arch"`
	BuiltOn             struct {
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
		return strings.TrimSpace(string(b)), nil
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

package brewery

import (
	"encoding/json"
	"os/exec"
)

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
	return string(b), err
}

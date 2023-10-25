package brewery_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/maxmcd/brewery"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gopkg.in/dnaeon/go-vcr.v3/recorder"
)

func TestGetFormula(t *testing.T) {
	recorder := newRecorder(t)
	client := &http.Client{Transport: recorder}

	b, err := brewery.NewBrewery(brewery.OptionWithHTTPClient(client))
	if err != nil {
		t.Fatal(err)
	}

	formula, err := b.FetchFormula(context.Background(), "ruby")
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(formula.ManifestURL())
}

func TestManifestJson(t *testing.T) {
	f, err := os.Open("./testdata/ffmpeg-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest brewery.Manifest
	if err := json.NewDecoder(f).Decode(&manifest); err != nil {
		t.Fatal(err)
	}

	firstArch := manifest.Manifests[0].Annotations.ShBrewTab.Arch
	if firstArch == "" {
		t.Fatal("Tab wasn't unmarshalled with value")
	}
}

func dockerLocalhost() string {
	if runtime.GOOS == "darwin" {
		return "host.docker.internal"
	}
	return "127.0.0.1"
}

func newRecorder(t *testing.T) *recorder.Recorder {
	recorder, err := recorder.NewWithOptions(&recorder.Options{
		CassetteName: "brew-recorder",
		Mode:         recorder.ModeReplayOnly,
		// Mode: recorder.ModeRecordOnly,
		// SkipRequestLatency: true,
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

func TestProxy(t *testing.T) {
	recorder := newRecorder(t)

	proxyHost := ""
	// export HOMEBREW_ARTIFACT_DOMAIN=http://localhost:3456
	// export HOMEBREW_API_DOMAIN=http://localhost:3456
	proxy := &httputil.ReverseProxy{
		ModifyResponse: func(r *http.Response) error {
			v, _ := httputil.DumpResponse(r, false)
			fmt.Println("--------------------")
			fmt.Println(string(v))
			fmt.Println("--------------------")

			switch r.StatusCode {
			case http.StatusFound, http.StatusMovedPermanently, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
				l, err := r.Location()
				if err != nil {
					return fmt.Errorf("error reading location header: %w", err)
				}
				l.Host = proxyHost
				l.Scheme = "http"
				r.Header.Set("Location", l.String())
				fmt.Printf("Setting Location header to %q\n", l.String())
			}
			return nil
		},
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out = pr.In
			if strings.HasSuffix(pr.In.URL.Path, ".jws.json") {
				pr.Out.Host = "formulae.brew.sh"
				pr.Out.URL.Scheme = "https"
				pr.Out.URL.Path = "/api" + pr.Out.URL.Path
				pr.Out.URL.Host = "formulae.brew.sh"
			}
			fmt.Printf("%q\n", pr.In.URL.Path)
			if strings.HasPrefix(pr.In.URL.Path, "/v2/homebrew/core") {
				pr.Out.Host = "ghcr.io"
				pr.Out.URL.Scheme = "https"
				pr.Out.URL.Host = "ghcr.io"
			}
			if strings.HasPrefix(pr.In.URL.Path, "/ghcr1/blobs/") {
				pr.Out.Host = "pkg-containers.githubusercontent.com"
				pr.Out.URL.Scheme = "https"
				pr.Out.URL.Host = "pkg-containers.githubusercontent.com"
			}
			v, _ := httputil.DumpRequest(pr.Out, false)
			fmt.Println("--------------------")
			fmt.Println(string(v))
			fmt.Println("--------------------")

		},
		Transport: recorder,
	}
	server := httptest.NewServer(proxy)

	ctx := context.Background()

	u, _ := url.Parse(server.URL)
	_, port, _ := strings.Cut(u.Host, ":")
	u.Host = dockerLocalhost() + ":" + port
	proxyHost = u.Host
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			NetworkMode: "host",
			Image:       "homebrew/brew",
			// Cmd:   []string{"bash", "-c", "echo hi && sleep 100000"},
			Cmd: []string{"brew", "install", "-vd", "cowsay"},
			Env: map[string]string{
				"HOMEBREW_ARTIFACT_DOMAIN": u.String(),
				"HOMEBREW_API_DOMAIN":      u.String(),
				"HOMEBREW_NO_AUTO_UPDATE":  "true",
			},
			// WaitingFor: wait.ForLog("hi"),
			WaitingFor: wait.ForExit(),
		},
		Started: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = container

	logs, err := container.Logs(context.Background())
	if err != nil {
		panic(err)
	}
	_, _ = io.Copy(os.Stdout, logs)
	// _, logs, err := container.Exec(ctx, []string{"brew", "install", "-vd", "ruby"})
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// _, _ = io.Copy(os.Stdout, logs)
	// fmt.Println(time.Since(start))
}

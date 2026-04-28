package skills

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

var (
	ErrInvalidGitHubSpec   = errors.New("github: invalid spec (expected github:owner/repo[@tag])")
	ErrPackageNotInstalled = errors.New("github: package not installed")
)

type GitHubSpec struct {
	Owner string
	Repo  string
	Tag   string
}

var gitHubSpecRE = regexp.MustCompile(`^github:([A-Za-z0-9](?:[A-Za-z0-9-]{0,37})?[A-Za-z0-9]|[A-Za-z0-9])/([A-Za-z0-9][A-Za-z0-9._-]*)(?:@([^\s\x00]{1,255}))?$`)

func ParseGitHubSpec(s string) (*GitHubSpec, error) {
	m := gitHubSpecRE.FindStringSubmatch(s)
	if m == nil {
		return nil, ErrInvalidGitHubSpec
	}
	return &GitHubSpec{Owner: m[1], Repo: m[2], Tag: m[3]}, nil
}

type GitHubPackagesConfig struct {
	BinDir       string
	ManifestPath string
}

func (c *GitHubPackagesConfig) Defaults() {
	if c.BinDir == "" {
		c.BinDir = "/app/data/.runtime/bin"
	}
	if c.ManifestPath == "" {
		c.ManifestPath = filepath.Join(filepath.Dir(c.BinDir), "github-packages.json")
	}
}

type GitHubPackageEntry struct {
	Name        string    `json:"name"`
	Repo        string    `json:"repo"`
	Tag         string    `json:"tag"`
	Binaries    []string  `json:"binaries"`
	InstalledAt time.Time `json:"installed_at"`
}

type GitHubInstaller struct {
	Config *GitHubPackagesConfig
	mu     sync.Mutex
}

func NewGitHubInstaller(cfg *GitHubPackagesConfig) *GitHubInstaller {
	if cfg == nil {
		cfg = &GitHubPackagesConfig{}
	}
	cfg.Defaults()
	return &GitHubInstaller{Config: cfg}
}

func (i *GitHubInstaller) Install(ctx context.Context, spec string) (*GitHubPackageEntry, error) {
	_ = ctx
	parsed, err := ParseGitHubSpec(spec)
	if err != nil {
		return nil, err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	entries, _ := i.readManifest()
	entry := GitHubPackageEntry{
		Name:        parsed.Repo,
		Repo:        parsed.Owner + "/" + parsed.Repo,
		Tag:         parsed.Tag,
		Binaries:    []string{parsed.Repo},
		InstalledAt: time.Now().UTC(),
	}
	var out []GitHubPackageEntry
	replaced := false
	for _, e := range entries {
		if e.Name == entry.Name {
			out = append(out, entry)
			replaced = true
			continue
		}
		out = append(out, e)
	}
	if !replaced {
		out = append(out, entry)
	}
	if err := i.writeManifest(out); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (i *GitHubInstaller) Uninstall(ctx context.Context, name string) error {
	_ = ctx
	i.mu.Lock()
	defer i.mu.Unlock()
	entries, _ := i.readManifest()
	var out []GitHubPackageEntry
	removed := false
	for _, e := range entries {
		if e.Name == name {
			removed = true
			continue
		}
		out = append(out, e)
	}
	if !removed {
		return ErrPackageNotInstalled
	}
	return i.writeManifest(out)
}

func (i *GitHubInstaller) List() ([]GitHubPackageEntry, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.readManifest()
}

func (i *GitHubInstaller) readManifest() ([]GitHubPackageEntry, error) {
	data, err := os.ReadFile(i.Config.ManifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var entries []GitHubPackageEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (i *GitHubInstaller) writeManifest(entries []GitHubPackageEntry) error {
	if err := os.MkdirAll(filepath.Dir(i.Config.ManifestPath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(i.Config.ManifestPath, data, 0644)
}

func init() {
	SetDefaultGitHubInstaller(NewGitHubInstaller(nil))
}

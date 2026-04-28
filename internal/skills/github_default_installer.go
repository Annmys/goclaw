package skills

import "sync"

var (
	defaultGitHubInstallerMu sync.RWMutex
	defaultGitHubInstaller   *GitHubInstaller
)

func SetDefaultGitHubInstaller(i *GitHubInstaller) {
	defaultGitHubInstallerMu.Lock()
	defer defaultGitHubInstallerMu.Unlock()
	defaultGitHubInstaller = i
}

func DefaultGitHubInstaller() *GitHubInstaller {
	defaultGitHubInstallerMu.RLock()
	defer defaultGitHubInstallerMu.RUnlock()
	return defaultGitHubInstaller
}

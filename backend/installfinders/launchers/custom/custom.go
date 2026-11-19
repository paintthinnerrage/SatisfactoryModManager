package custom

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/viper"

	"github.com/satisfactorymodding/SatisfactoryModManager/backend/installfinders/common"
)

const (
	// ConfigFileName is the custom install list, stored next to settings.json.
	ConfigFileName      = "custom-installs.json"
	defaultLauncherName = "Custom"
	windowsOS           = "windows"
)

// ConfigPath is custom-installs.json in SMM's config directory (smm-local-dir).
func ConfigPath() string {
	return filepath.Join(viper.GetString("smm-local-dir"), ConfigFileName)
}

type configFile struct {
	Installs []installEntry `json:"installs"`
}

type installEntry struct {
	Path   string
	Branch string
	Name   string
}

func (e *installEntry) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return fmt.Errorf("empty install entry")
	}

	switch data[0] {
	case '"':
		var path string
		if err := json.Unmarshal(data, &path); err != nil {
			return fmt.Errorf("invalid install path: %w", err)
		}
		e.Path = path
		return nil
	case '{':
		var obj struct {
			Path   string `json:"path"`
			Branch string `json:"branch"`
			Name   string `json:"name"`
		}
		if err := json.Unmarshal(data, &obj); err != nil {
			return fmt.Errorf("invalid install entry: %w", err)
		}
		*e = installEntry{Path: obj.Path, Branch: obj.Branch, Name: obj.Name}
		return nil
	default:
		return fmt.Errorf("install entry must be a path string or an object with a path")
	}
}

// FindInstallations loads user-configured Satisfactory install paths.
// A missing config file means the user has not set any custom paths.
func FindInstallations() ([]*common.Installation, []error) {
	return FindInstallationsIn(ConfigPath())
}

// FindInstallationsIn loads custom installs from configPath.
func FindInstallationsIn(configPath string) ([]*common.Installation, []error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("no custom installs config", slog.String("path", configPath))
			return nil, nil
		}
		return nil, []error{reject(configPath, fmt.Errorf("failed to read custom installs config: %w", err))}
	}

	cfg, err := parseConfig(data)
	if err != nil {
		return nil, []error{reject(configPath, err)}
	}
	if len(cfg.Installs) == 0 {
		slog.Debug("custom installs config has no paths", slog.String("path", configPath))
		return nil, nil
	}

	slog.Info("loading custom Satisfactory install paths", slog.String("config", configPath), slog.Int("count", len(cfg.Installs)))

	installs := make([]*common.Installation, 0, len(cfg.Installs))
	var findErrors []error
	seen := make([]string, 0, len(cfg.Installs))

	for _, entry := range cfg.Installs {
		install, err := installationFromEntry(entry)
		if err != nil {
			reportedPath := strings.TrimSpace(entry.Path)
			if reportedPath == "" {
				reportedPath = configPath
			}
			findErrors = append(findErrors, reject(reportedPath, err))
			continue
		}
		if pathSeen(seen, install.Path) {
			slog.Warn("duplicate custom install path ignored", slog.String("config", configPath), slog.String("path", install.Path))
			continue
		}
		seen = append(seen, install.Path)
		installs = append(installs, install)
		slog.Info(
			"using custom Satisfactory install",
			slog.String("path", install.Path),
			slog.String("name", install.Launcher),
			slog.String("type", string(install.Type)),
			slog.String("branch", string(install.Branch)),
			slog.Int("version", install.Version),
		)
	}

	slog.Info(
		"finished loading custom Satisfactory installs",
		slog.String("config", configPath),
		slog.Int("accepted", len(installs)),
		slog.Int("rejected", len(findErrors)),
	)

	return installs, findErrors
}

func parseConfig(data []byte) (configFile, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return configFile{}, nil
	}

	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return configFile{}, fmt.Errorf("failed to parse custom installs config: %w", err)
	}
	if _, ok := probe["installs"]; !ok {
		if len(probe) == 0 {
			return configFile{}, nil
		}
		return configFile{}, fmt.Errorf("custom installs config must contain an \"installs\" array of paths")
	}

	var cfg configFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return configFile{}, fmt.Errorf("failed to parse custom installs config: %w", err)
	}
	return cfg, nil
}

func installationFromEntry(entry installEntry) (*common.Installation, error) {
	resolved, err := resolvePath(entry.Path)
	if err != nil {
		return nil, err
	}
	if err := validateInstallDir(resolved); err != nil {
		return nil, fmt.Errorf("%s: %w", resolved, err)
	}

	details, err := common.ReadGameDetails(resolved, common.NativePlatform())
	if err != nil {
		if errors.Is(err, common.ErrUnrecognizedGame) {
			return nil, fmt.Errorf("%s: not a Satisfactory installation: missing a known game executable (%s) and its version file", resolved, strings.Join(common.KnownGameExecutables(), ", "))
		}
		return nil, fmt.Errorf("%s: failed to read Satisfactory installation: %w", resolved, err)
	}

	branch, err := resolveBranch(entry.Branch, details.BranchName)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(entry.Name)
	if name == "" {
		name = defaultLauncherName
	}

	return &common.Installation{
		Path:       resolved,
		Version:    details.Version,
		Type:       details.Type,
		Location:   common.LocationTypeLocal,
		Branch:     branch,
		Launcher:   name,
		LaunchPath: launchCommand(details.Executable, details.Type),
		SavedPath:  details.SavedPath,
	}, nil
}

func resolvePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("install path is empty")
	}

	expanded, err := expandHome(path)
	if err != nil {
		return "", err
	}

	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("failed to resolve install path %s: %w", path, err)
	}
	return abs, nil
}

func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to expand ~ in install path: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

func validateInstallDir(path string) error {
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist")
		}
		return fmt.Errorf("failed to read directory: %w", err)
	}
	if !stat.IsDir() {
		return fmt.Errorf("path is not a directory")
	}
	return nil
}

func resolveBranch(configured string, branchName string) (common.GameBranch, error) {
	switch strings.ToLower(strings.TrimSpace(configured)) {
	case "":
		if strings.Contains(strings.ToLower(branchName), "experimental") {
			return common.BranchExperimental, nil
		}
		return common.BranchStable, nil
	case "stable":
		return common.BranchStable, nil
	case "experimental":
		return common.BranchExperimental, nil
	default:
		return "", fmt.Errorf("unknown branch %q (expected %q or %q)", configured, common.BranchStable, common.BranchExperimental)
	}
}

func launchCommand(executable string, installType common.InstallType) []string {
	switch runtime.GOOS {
	case windowsOS:
		if installType == common.InstallTypeLinuxServer {
			return nil
		}
		// start returns immediately, matching the other Windows launcher commands.
		return []string{"cmd", "/C", "start", "", executable}
	default:
		if installType == common.InstallTypeLinuxServer {
			return []string{executable}
		}
		return nil
	}
}

func pathSeen(paths []string, path string) bool {
	for _, existing := range paths {
		if common.OsPathEqual(existing, path) {
			return true
		}
	}
	return false
}

func reject(path string, err error) error {
	slog.Error("custom Satisfactory install path was rejected", slog.String("path", path), slog.Any("error", err))
	return common.InstallFindError{Path: path, Inner: err}
}

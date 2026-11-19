package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

type installType struct {
	versionPath string
	executable  string
	installType InstallType
}

var gameInfo = []installType{
	{
		executable:  "FactoryServer.sh",
		versionPath: filepath.Join("Engine", "Binaries", "Linux", "UnrealServer-Linux-Shipping.version"),
		installType: InstallTypeLinuxServer,
	},
	{
		executable:  "FactoryServer.exe",
		versionPath: filepath.Join("Engine", "Binaries", "Win64", "UnrealServer-Win64-Shipping.version"),
		installType: InstallTypeWindowsServer,
	},
	{
		executable:  "FactoryGame.exe",
		versionPath: filepath.Join("Engine", "Binaries", "Win64", "FactoryGame-Win64-Shipping.version"),
		installType: InstallTypeWindowsClient,
	},
	// Update 9 stuff below
	{
		executable:  "FactoryServer.sh",
		versionPath: filepath.Join("Engine", "Binaries", "Linux", "FactoryServer-Linux-Shipping.version"),
		installType: InstallTypeLinuxServer,
	},
	{
		executable:  "FactoryServer.exe",
		versionPath: filepath.Join("Engine", "Binaries", "Win64", "FactoryServer-Win64-Shipping.version"),
		installType: InstallTypeWindowsServer,
	},
	// Update 1.0 stuff
	{
		executable:  "FactoryGameSteam.exe",
		versionPath: filepath.Join("Engine", "Binaries", "Win64", "FactoryGameSteam-Win64-Shipping.version"),
		installType: InstallTypeWindowsClient,
	},
	{
		executable:  "FactoryGameEGS.exe",
		versionPath: filepath.Join("Engine", "Binaries", "Win64", "FactoryGameEGS-Win64-Shipping.version"),
		installType: InstallTypeWindowsClient,
	},
}

type GameVersionFile struct {
	MajorVersion         int    `json:"MajorVersion"`
	MinorVersion         int    `json:"MinorVersion"`
	PatchVersion         int    `json:"PatchVersion"`
	Changelist           int    `json:"Changelist"`
	CompatibleChangelist int    `json:"CompatibleChangelist"`
	IsLicenseeVersion    int    `json:"IsLicenseeVersion"`
	IsPromotedBuild      int    `json:"IsPromotedBuild"`
	BranchName           string `json:"BranchName"`
	BuildID              string `json:"BuildId"`
}

// ErrUnrecognizedGame is returned when a directory has none of the known Satisfactory executables and version files.
// The text matches the historical GetGameInfo error.
var ErrUnrecognizedGame = errors.New("failed to get game info")

// GameDetails is the metadata read from a Satisfactory install directory.
type GameDetails struct {
	Type       InstallType
	Version    int
	SavedPath  string
	Executable string
	BranchName string
}

// KnownGameExecutables returns the game binaries SMM can recognize, without duplicates.
func KnownGameExecutables() []string {
	names := make([]string, 0, len(gameInfo))
	seen := make(map[string]struct{}, len(gameInfo))
	for _, info := range gameInfo {
		if _, ok := seen[info.executable]; ok {
			continue
		}
		seen[info.executable] = struct{}{}
		names = append(names, info.executable)
	}
	return names
}

func GetGameInfo(path string, platform Platform) (InstallType, int, string, error) {
	details, err := ReadGameDetails(path, platform)
	if err != nil {
		return InstallTypeWindowsClient, 0, "", err
	}
	return details.Type, details.Version, details.SavedPath, nil
}

// ReadGameDetails reports the install type, changelist, save directory, executable, and version-file branch name.
func ReadGameDetails(path string, platform Platform) (GameDetails, error) {
	for _, info := range gameInfo {
		executablePath := filepath.Join(path, info.executable)
		if _, err := os.Stat(executablePath); os.IsNotExist(err) {
			slog.Debug("game not of type", slog.String("path", executablePath), slog.String("type", string(info.installType)))
			continue
		}

		versionFilePath := filepath.Join(path, info.versionPath)
		if _, err := os.Stat(versionFilePath); os.IsNotExist(err) {
			slog.Debug("game not of type", slog.String("path", executablePath), slog.String("type", string(info.installType)))
			continue
		}

		versionFile, err := os.ReadFile(versionFilePath)
		if err != nil {
			return GameDetails{}, fmt.Errorf("failed to read version file %s: %w", versionFilePath, err)
		}

		var versionData GameVersionFile
		if err := json.Unmarshal(versionFile, &versionData); err != nil {
			return GameDetails{}, fmt.Errorf("failed to parse version file %s: %w", versionFilePath, err)
		}

		return GameDetails{
			Type:       info.installType,
			Version:    versionData.Changelist,
			SavedPath:  getGameSavedDir(path, info.installType, platform),
			Executable: executablePath,
			BranchName: versionData.BranchName,
		}, nil
	}
	return GameDetails{}, ErrUnrecognizedGame
}

func getGameSavedDir(gamePath string, install InstallType, platform Platform) string {
	if install == InstallTypeWindowsClient {
		cacheDir, err := platform.CacheDir()
		if err != nil {
			slog.Error("failed to get cache dir", slog.Any("error", err))
			return ""
		}

		return filepath.Join(cacheDir, "FactoryGame", "Saved")
	}
	return filepath.Join(gamePath, "FactoryGame", "Saved")
}

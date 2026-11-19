package installfinders

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"

	"github.com/satisfactorymodding/SatisfactoryModManager/backend/installfinders/common"
	"github.com/satisfactorymodding/SatisfactoryModManager/backend/installfinders/launchers/custom"
)

func TestMergeCustomInstallsKeepsAutomaticMatch(t *testing.T) {
	game := filepath.Join(t.TempDir(), "Satisfactory")
	writeClientInstall(t, game)
	withCustomConfig(t, []string{game})

	existing := []*common.Installation{{
		Path:     game,
		Launcher: "Steam",
		Branch:   common.BranchStable,
		Type:     common.InstallTypeWindowsClient,
		Location: common.LocationTypeLocal,
	}}
	got, errs := mergeCustomInstalls(existing, nil)
	if len(errs) != 0 {
		t.Fatalf("errs=%v", errs)
	}
	if len(got) != 1 {
		t.Fatalf("got %#v", got)
	}
	if got[0].Launcher != "Steam" {
		t.Fatalf("launcher = %s", got[0].Launcher)
	}
}

func TestMergeCustomInstallsAddsPathWhenDetectionMissesIt(t *testing.T) {
	game := filepath.Join(t.TempDir(), "Satisfactory")
	writeClientInstall(t, game)
	withCustomConfig(t, []string{game})

	got, errs := mergeCustomInstalls(nil, nil)
	if len(errs) != 0 {
		t.Fatalf("errs=%v", errs)
	}
	if len(got) != 1 {
		t.Fatalf("got %#v", got)
	}
	if got[0].Launcher != "Custom" {
		t.Fatalf("launcher = %s", got[0].Launcher)
	}
	if !common.OsPathEqual(got[0].Path, game) {
		t.Fatalf("path = %s", got[0].Path)
	}
	if got[0].Version != 111 {
		t.Fatalf("version = %d", got[0].Version)
	}
}

func TestFindInstallationsIncludesCustomPath(t *testing.T) {
	game := filepath.Join(t.TempDir(), "Satisfactory")
	writeClientInstall(t, game)
	withCustomConfig(t, []string{game})

	installs, _ := FindInstallations()
	for _, install := range installs {
		if common.OsPathEqual(install.Path, game) {
			if install.Version != 111 {
				t.Fatalf("version = %d", install.Version)
			}
			return
		}
	}
	t.Fatalf("custom install %s missing from %#v", game, installs)
}

func withCustomConfig(t *testing.T, paths []string) {
	t.Helper()
	dir := t.TempDir()
	previous := viper.GetString("smm-local-dir")
	viper.Set("smm-local-dir", dir)
	t.Cleanup(func() {
		viper.Set("smm-local-dir", previous)
	})

	payload, err := json.Marshal(map[string]any{"installs": paths})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, custom.ConfigFileName), payload, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeClientInstall(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "FactoryGame.exe"), []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	versionPath := filepath.Join(root, "Engine", "Binaries", "Win64", "FactoryGame-Win64-Shipping.version")
	if err := os.MkdirAll(filepath.Dir(versionPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(versionPath, []byte(`{"Changelist":111,"BranchName":"main"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

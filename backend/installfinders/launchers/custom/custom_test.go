package custom

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/satisfactorymodding/SatisfactoryModManager/backend/installfinders/common"
)

func TestMissingConfigIsNotAnError(t *testing.T) {
	installs, errs := FindInstallationsIn(filepath.Join(t.TempDir(), "custom-installs.json"))
	if installs != nil || errs != nil {
		t.Fatalf("missing config: installs=%v errs=%v", installs, errs)
	}
}

func TestEmptyConfigIsNotAnError(t *testing.T) {
	for _, contents := range []string{"", "   ", "{}", `{"installs":[]}`, `{"installs":null}`} {
		path := writeConfig(t, contents)
		installs, errs := FindInstallationsIn(path)
		if installs != nil || errs != nil {
			t.Fatalf("contents %q: installs=%v errs=%v", contents, installs, errs)
		}
	}
}

func TestInvalidConfigIsRejected(t *testing.T) {
	path := writeConfig(t, `{"paths":["/tmp/Satisfactory"]}`)
	installs, errs := FindInstallationsIn(path)
	if len(installs) != 0 || len(errs) != 1 {
		t.Fatalf("installs=%v errs=%v", installs, errs)
	}
	findErr := mustInstallFindError(t, errs[0])
	if findErr.Path != path {
		t.Fatalf("error path = %s", findErr.Path)
	}
	if !strings.Contains(findErr.Inner.Error(), `"installs"`) {
		t.Fatalf("error = %v", findErr.Inner)
	}
}

func TestMalformedEntryRejectsTheFile(t *testing.T) {
	path := writeConfig(t, `{"installs":[1]}`)
	_, errs := FindInstallationsIn(path)
	if len(errs) != 1 {
		t.Fatalf("errs=%v", errs)
	}
	findErr := mustInstallFindError(t, errs[0])
	if findErr.Path != path {
		t.Fatalf("error path = %s", findErr.Path)
	}
}

func TestValidClientInstall(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "Satisfactory")
	writeClient(t, game, "++FactoryGame+rel-main", 385279)

	configPath := writeConfig(t, `{"installs":[{"path":"`+jsonEscape(game)+`","name":"Manual"}]}`)
	installs, errs := FindInstallationsIn(configPath)
	if len(errs) != 0 {
		t.Fatalf("errs=%v", errs)
	}
	if len(installs) != 1 {
		t.Fatalf("installs=%v", installs)
	}

	got := installs[0]
	if got.Path != game {
		t.Fatalf("path = %s", got.Path)
	}
	if got.Version != 385279 {
		t.Fatalf("version = %d", got.Version)
	}
	if got.Type != common.InstallTypeWindowsClient {
		t.Fatalf("type = %s", got.Type)
	}
	if got.Branch != common.BranchStable {
		t.Fatalf("branch = %s", got.Branch)
	}
	if got.Launcher != "Manual" {
		t.Fatalf("launcher = %s", got.Launcher)
	}
	if got.Location != common.LocationTypeLocal {
		t.Fatalf("location = %s", got.Location)
	}
	if runtime.GOOS == windowsOS {
		want := []string{"cmd", "/C", "start", "", filepath.Join(game, "FactoryGame.exe")}
		if !reflect.DeepEqual(got.LaunchPath, want) {
			t.Fatalf("launchPath = %#v", got.LaunchPath)
		}
	} else if got.LaunchPath != nil {
		t.Fatalf("windows client launchPath = %#v", got.LaunchPath)
	}
}

func TestStringEntryAndExperimentalBranch(t *testing.T) {
	game := filepath.Join(t.TempDir(), "server")
	writeLinuxServer(t, game, "++FactoryGame+experimental", 400000)

	configPath := writeConfig(t, `{"installs":["`+jsonEscape(game)+`"]}`)
	installs, errs := FindInstallationsIn(configPath)
	if len(errs) != 0 {
		t.Fatalf("errs=%v", errs)
	}
	if len(installs) != 1 {
		t.Fatalf("installs=%v", installs)
	}
	got := installs[0]
	if got.Type != common.InstallTypeLinuxServer {
		t.Fatalf("type = %s", got.Type)
	}
	if got.Branch != common.BranchExperimental {
		t.Fatalf("branch = %s", got.Branch)
	}
	if got.Launcher != defaultLauncherName {
		t.Fatalf("launcher = %s", got.Launcher)
	}
	if got.Version != 400000 {
		t.Fatalf("version = %d", got.Version)
	}
	if runtime.GOOS == windowsOS {
		if got.LaunchPath != nil {
			t.Fatalf("linux server launchPath = %#v", got.LaunchPath)
		}
	} else if !reflect.DeepEqual(got.LaunchPath, []string{filepath.Join(game, "FactoryServer.sh")}) {
		t.Fatalf("launchPath = %#v", got.LaunchPath)
	}
}

func TestExplicitBranchOverridesVersionFile(t *testing.T) {
	game := filepath.Join(t.TempDir(), "game")
	writeClient(t, game, "experimental-preview", 1)
	configPath := writeConfig(t, `{"installs":[{"path":"`+jsonEscape(game)+`","branch":"stable"}]}`)
	installs, errs := FindInstallationsIn(configPath)
	if len(errs) != 0 || len(installs) != 1 {
		t.Fatalf("installs=%v errs=%v", installs, errs)
	}
	if installs[0].Branch != common.BranchStable {
		t.Fatalf("branch = %s", installs[0].Branch)
	}
}

func TestInvalidPathsAreReportedAndValidOnesAreKept(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	writeClient(t, game, "main", 42)
	missing := filepath.Join(root, "missing")
	notAGame := filepath.Join(root, "notes")
	if err := os.MkdirAll(notAGame, 0o755); err != nil {
		t.Fatal(err)
	}
	aFile := filepath.Join(root, "file.txt")
	if err := os.WriteFile(aFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	payload := map[string]any{
		"installs": []any{
			game,
			missing,
			notAGame,
			aFile,
			map[string]string{"path": game, "name": "duplicate"},
			map[string]string{"path": filepath.Join(root, "bad-branch"), "branch": "beta"},
			map[string]string{"path": "  "},
		},
	}
	writeClient(t, filepath.Join(root, "bad-branch"), "main", 1)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	configPath := writeConfig(t, string(encoded))

	installs, errs := FindInstallationsIn(configPath)
	if len(installs) != 1 {
		t.Fatalf("installs=%v errs=%v", installs, errs)
	}
	if installs[0].Path != game || installs[0].Launcher != defaultLauncherName {
		t.Fatalf("kept %+v", installs[0])
	}
	if len(errs) != 5 {
		t.Fatalf("got %d errors: %v", len(errs), errs)
	}
	for _, err := range errs {
		findErr := mustInstallFindError(t, err)
		if findErr.Path == "" || findErr.Inner == nil || findErr.Error() == "" {
			t.Fatalf("unclear error: %#v", findErr)
		}
	}
}

func TestHomeExpansion(t *testing.T) {
	home := t.TempDir()
	if runtime.GOOS == windowsOS {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("HOME", home)
	}
	game := filepath.Join(home, "Games", "Satisfactory")
	writeClient(t, game, "main", 7)

	configPath := writeConfig(t, `{"installs":["~/Games/Satisfactory"]}`)
	installs, errs := FindInstallationsIn(configPath)
	if len(errs) != 0 || len(installs) != 1 {
		t.Fatalf("installs=%v errs=%v", installs, errs)
	}
	if !common.OsPathEqual(installs[0].Path, game) {
		t.Fatalf("path = %s, want %s", installs[0].Path, game)
	}
}

func TestCorruptVersionFile(t *testing.T) {
	game := filepath.Join(t.TempDir(), "game")
	writeClient(t, game, "main", 1)
	versionPath := filepath.Join(game, "Engine", "Binaries", "Win64", "FactoryGame-Win64-Shipping.version")
	if err := os.WriteFile(versionPath, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := writeConfig(t, `{"installs":["`+jsonEscape(game)+`"]}`)
	installs, errs := FindInstallationsIn(configPath)
	if len(installs) != 0 || len(errs) != 1 {
		t.Fatalf("installs=%v errs=%v", installs, errs)
	}
	if !strings.Contains(errs[0].Error(), "failed to parse version file") {
		t.Fatalf("error = %v", errs[0])
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ConfigFileName)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeClient(t *testing.T, root string, branchName string, changelist int) {
	t.Helper()
	writeGame(t, root, "FactoryGame.exe", filepath.Join("Engine", "Binaries", "Win64", "FactoryGame-Win64-Shipping.version"), branchName, changelist)
}

func writeLinuxServer(t *testing.T, root string, branchName string, changelist int) {
	t.Helper()
	writeGame(t, root, "FactoryServer.sh", filepath.Join("Engine", "Binaries", "Linux", "UnrealServer-Linux-Shipping.version"), branchName, changelist)
}

func writeGame(t *testing.T, root string, executable string, versionRel string, branchName string, changelist int) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, executable), []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	versionPath := filepath.Join(root, versionRel)
	if err := os.MkdirAll(filepath.Dir(versionPath), 0o755); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"Changelist": changelist,
		"BranchName": branchName,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(versionPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustInstallFindError(t *testing.T, err error) common.InstallFindError {
	t.Helper()
	var findErr common.InstallFindError
	if !errors.As(err, &findErr) {
		t.Fatalf("error type = %T (%v)", err, err)
	}
	return findErr
}

func jsonEscape(path string) string {
	encoded, err := json.Marshal(path)
	if err != nil {
		panic(err)
	}
	return string(encoded[1 : len(encoded)-1])
}

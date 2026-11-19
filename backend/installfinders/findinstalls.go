package installfinders

import (
	"log/slog"
	"strings"

	"golang.org/x/exp/maps"

	"github.com/satisfactorymodding/SatisfactoryModManager/backend/installfinders/common"
	"github.com/satisfactorymodding/SatisfactoryModManager/backend/installfinders/launchers"
	"github.com/satisfactorymodding/SatisfactoryModManager/backend/installfinders/launchers/custom"

	_ "github.com/satisfactorymodding/SatisfactoryModManager/backend/installfinders/launchers/all" // register all launchers
)

func FindInstallations() ([]*common.Installation, []error) {
	registrations := launchers.GetInstallFinders()

	slog.Debug("finding installations", slog.String("launchers", strings.Join(maps.Keys(registrations), ",")))

	installs, findErrors := common.FindAll(maps.Values(registrations)...)
	return mergeCustomInstalls(installs, findErrors)
}

// mergeCustomInstalls appends user-configured installs that automatic detection did not already return.
// Automatic matches keep their launcher metadata when the same directory is also listed in the config.
func mergeCustomInstalls(installs []*common.Installation, findErrors []error) ([]*common.Installation, []error) {
	customInstalls, customErrors := custom.FindInstallations()
	findErrors = append(findErrors, customErrors...)

	for _, install := range customInstalls {
		if containsInstallPath(installs, install.Path) {
			slog.Info("custom install path is already detected", slog.String("path", install.Path))
			continue
		}
		installs = append(installs, install)
	}
	return installs, findErrors
}

func containsInstallPath(installs []*common.Installation, path string) bool {
	for _, install := range installs {
		if common.OsPathEqual(install.Path, path) {
			return true
		}
	}
	return false
}

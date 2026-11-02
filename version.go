package gorest

import "runtime/debug"

const gorestModulePath = "github.com/elsyahtech/gorest"

func currentGORESTVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return developmentGORESTVersion
	}

	return gorestVersionFromBuildInfo(info)
}

func gorestVersionFromBuildInfo(info *debug.BuildInfo) string {
	if info == nil {
		return developmentGORESTVersion
	}

	if info.Main.Path == gorestModulePath {
		return releaseGORESTVersion(info.Main.Version)
	}

	for _, dependency := range info.Deps {
		if dependency.Path == gorestModulePath {
			if dependency.Replace != nil {
				return releaseGORESTVersion(dependency.Replace.Version)
			}

			return releaseGORESTVersion(dependency.Version)
		}
	}

	return developmentGORESTVersion
}

const developmentGORESTVersion = "development (untagged)"

func releaseGORESTVersion(version string) string {
	if version == "" || version == "(devel)" {
		return developmentGORESTVersion
	}

	return version
}

package main

import "runtime/debug"

var version = "dev"

// readBuildInfo is replaced in tests to exercise each build-info outcome.
var readBuildInfo = debug.ReadBuildInfo

func cliVersion() string {
	if version != "" && version != "dev" {
		return version
	}

	info, ok := readBuildInfo()
	if !ok {
		return version
	}
	if info.Main.Version == "" || info.Main.Version == "(devel)" {
		return version
	}

	return info.Main.Version
}

package main

import (
	"strings"

	"github.com/coreos/go-semver/semver"
)

func IsVersionLabel(label string) bool {
	_, err := ParseSemVer(label)
	return err == nil
}

func ParseSemVer(version string) (*semver.Version, error) {
	// strip leading 'v' if present
	if version[0] == 'v' || version[0] == 'V' {
		version = version[1:]
	}

	// add missing patch version
	if strings.Count(version, ".") == 0 {
		version = version + ".0.0"
	}

	// add missing minor and patch version
	if strings.Count(version, ".") == 1 {
		version = version + ".0"
	}

	return semver.NewVersion(version)
}

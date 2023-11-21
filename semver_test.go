package main

import (
	"testing"

	"github.com/coreos/go-semver/semver"
)

func TestSemVerParse(t *testing.T) {
	inputs := []struct {
		input    string
		expected semver.Version
	}{
		{input: "1.2.3", expected: semver.Version{Major: 1, Minor: 2, Patch: 3}},
		{input: "v1.2.3", expected: semver.Version{Major: 1, Minor: 2, Patch: 3}},
		{input: "1.2.3-alpha", expected: semver.Version{Major: 1, Minor: 2, Patch: 3, PreRelease: semver.PreRelease("alpha")}},
		{input: "1.2.3-alpha.1", expected: semver.Version{Major: 1, Minor: 2, Patch: 3, PreRelease: semver.PreRelease("alpha.1")}},
		{input: "1.2.3-0.3.7", expected: semver.Version{Major: 1, Minor: 2, Patch: 3, PreRelease: semver.PreRelease("0.3.7")}},
		{input: "1.2.3-x.7.z.92", expected: semver.Version{Major: 1, Minor: 2, Patch: 3, PreRelease: semver.PreRelease("x.7.z.92")}},
		{input: "1.2.3-x-y-z.-", expected: semver.Version{Major: 1, Minor: 2, Patch: 3, PreRelease: semver.PreRelease("x-y-z.-")}},
		{input: "1.2.3-x-y-z+", expected: semver.Version{Major: 1, Minor: 2, Patch: 3, PreRelease: semver.PreRelease("x-y-z")}},
		{input: "1.2.3-x-y-z+metadata", expected: semver.Version{Major: 1, Minor: 2, Patch: 3, PreRelease: semver.PreRelease("x-y-z"), Metadata: "metadata"}},
		{input: "1.2.3+metadata", expected: semver.Version{Major: 1, Minor: 2, Patch: 3, Metadata: "metadata"}},
		{input: "1.2.3-rc.1+metadata", expected: semver.Version{Major: 1, Minor: 2, Patch: 3, PreRelease: semver.PreRelease("rc.1"), Metadata: "metadata"}},
		{input: "1.2.3-rc.1+metadata.2", expected: semver.Version{Major: 1, Minor: 2, Patch: 3, PreRelease: semver.PreRelease("rc.1"), Metadata: "metadata.2"}},
		{input: "1.2.3-rc.1+metadata.2.3", expected: semver.Version{Major: 1, Minor: 2, Patch: 3, PreRelease: semver.PreRelease("rc.1"), Metadata: "metadata.2.3"}},
		{input: "1.2.3-rc.1+metadata.2.3.4", expected: semver.Version{Major: 1, Minor: 2, Patch: 3, PreRelease: semver.PreRelease("rc.1"), Metadata: "metadata.2.3.4"}},
		{input: "1", expected: semver.Version{Major: 1, Minor: 0, Patch: 0}},
		{input: "1.0", expected: semver.Version{Major: 1, Minor: 0, Patch: 0}},
		{input: "v2", expected: semver.Version{Major: 2, Minor: 0, Patch: 0}},
		{input: "v2.3", expected: semver.Version{Major: 2, Minor: 3, Patch: 0}},
	}

	// test inputs
	for _, input := range inputs {
		v, err := ParseSemVer(input.input)
		if err != nil {
			t.Errorf("failed to parse version: %v", err)
		}
		if *v != input.expected {
			t.Errorf("expected %v, got %v", input.expected, v)
		}
	}

	//_, err := semver.NewVersion("1.2.3")
	//if err != nil {
	//	t.Errorf("failed to parse version: %v", err)
	//}
}

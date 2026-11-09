package main

import (
	"errors"
	"log"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/coreos/go-semver/semver"
)

var (
	DefaultReleaseInfoKeyExtractor = MustReleaseDetailsKeyExtractor(`(?P<Prefix>.*?/)?(?P<Product>[^/]+)/(?P<Version>[^/]+)/(?P<PackageName>[^_]+)(_|_(?P<Extra>.*?)_)(?P<OS>[^_\d]+)_(?P<Arch>[^\.]+)\.(?P<ArchiveType>[^/]+)$`)
)

// ReleaseDetailsKeyExtractor holds a pre-compiled regex used to extract release
// metadata from S3 object keys. Compiling once at construction avoids per-object
// regexp.Compile calls which dominated memory allocation under concurrent rendering.
type ReleaseDetailsKeyExtractor struct {
	re *regexp.Regexp
}

// MustReleaseDetailsKeyExtractor compiles pattern and panics if it is invalid.
// Intended for package-level variables with known-good patterns.
func MustReleaseDetailsKeyExtractor(pattern string) ReleaseDetailsKeyExtractor {
	return ReleaseDetailsKeyExtractor{re: regexp.MustCompile(pattern)}
}

// NewReleaseDetailsKeyExtractor compiles pattern and returns an error if invalid.
// Use this for patterns supplied at runtime (e.g. RELEASE_KEY_PATTERNS env var).
func NewReleaseDetailsKeyExtractor(pattern string) (ReleaseDetailsKeyExtractor, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ReleaseDetailsKeyExtractor{}, err
	}

	return ReleaseDetailsKeyExtractor{re: re}, nil
}

func extractMetadataFromKey(re *regexp.Regexp, key string) (map[string]string, error) {
	if !re.MatchString(key) {
		return nil, errors.New("failed to match")
	}

	match := re.FindStringSubmatch(key)
	names := re.SubexpNames()
	results := make(map[string]string, len(names))
	for i, name := range names {
		if name == "" {
			continue
		}
		results[name] = match[i]
	}

	return results, nil
}

func (k ReleaseDetailsKeyExtractor) ExtractReleaseDetails(key string) (map[string]string, error) {
	results, err := extractMetadataFromKey(k.re, key)
	if err != nil {
		return nil, err
	}

	versionStr := results["Version"]
	if ver, err := ParseSemVer(versionStr); err == nil {
		results["Version"] = ver.String()
	}

	return results, nil
}

type ReleaseDetailKeyExtractions []ReleaseDetailsKeyExtractor

func (k ReleaseDetailKeyExtractions) ExtractReleaseDetails(key string) (map[string]string, error) {
	for _, extractor := range k {
		if results, err := extractor.ExtractReleaseDetails(key); err == nil {
			return results, nil
		}
	}

	return nil, errors.New("failed to match")
}

type IndexConfig struct {
	KeyExtractions ReleaseDetailKeyExtractions // `.*?/(?P<Product>[^/]+)/(?P<Version>[^/]+)/(?P<PackageName>[^_]+)(_|_(?P<Extra>.*?)_)(?P<OS>[^_\d]+)_(?P<Arch>[^\.]+)\.(?P<ArchiveType>[^/]+)$`
}

type ArchiveIndex struct {
	Product map[string]*ProductIndex `json:"product,omitempty"`
}

func (a *ArchiveIndex) AddProduct(p *ProductIndex) {
	a.Product[p.Name] = p
}

func NewArchiveIndex() *ArchiveIndex {
	return &ArchiveIndex{
		Product: make(map[string]*ProductIndex),
	}
}

type ProductIndex struct {
	Name string `json:"name"`

	// Versions List of all product versions
	Versions map[string]*VersionIndex `json:"versions,omitempty"`

	// LatestReleases is the latest version of each major version, keyed by the major number.
	LatestReleases map[string]*VersionIndex `json:"releases,omitempty"`

	// LatestVersion Latest release of the product
	LatestVersion *VersionIndex `json:"latest,omitempty"`

	versions []*semver.Version
}

func (p *ProductIndex) AddVersion(v *VersionIndex) {
	p.Versions[v.Version] = v

	sv, err := semver.NewVersion(v.Version)
	if err == nil {
		p.versions = append(p.versions, sv)
		semver.Sort(p.versions)
		p.LatestVersion = p.Versions[p.versions[len(p.versions)-1].String()]
		p.LatestReleases[strconv.FormatInt(sv.Major, 10)] = p.Versions[p.latestOfMajor(sv.Major).String()]
	}
}

// latestOfMajor returns the highest known version with the given major number. versions is sorted
// and holds at least one version of that major when this is called.
func (p *ProductIndex) latestOfMajor(major int64) *semver.Version {
	for _, v := range slices.Backward(p.versions) {
		if v.Major == major {
			return v
		}
	}

	return nil
}

func NewProductIndex(name string) *ProductIndex {
	return &ProductIndex{
		Name:           name,
		Versions:       make(map[string]*VersionIndex),
		LatestReleases: make(map[string]*VersionIndex),
		versions:       make([]*semver.Version, 0),
	}
}

type VersionIndex struct {
	Builds []*IndexEntry `json:"builds,omitempty"`

	Name    string `json:"name,omitempty"`
	Shasums string `json:"shasums,omitempty"`
	//	ShasumsSignature  string   `json:"shasums_signature"`
	//	ShasumsSignatures []string `json:"shasums_signatures"`
	Version string `json:"version,omitempty"`
	// CommitSha          string   `json:"commit"`
}

func (v *VersionIndex) AddBuild(build *IndexEntry) {
	v.Builds = append(v.Builds, build)
}

// addObject records o as a build, or as the shasums file, and reports whether it was recognised.
func (v *VersionIndex) addObject(cfg IndexConfig, o Object) bool {
	isShasums := strings.HasSuffix(o.Key(), "_SHA256SUMS")
	if isShasums {
		v.Shasums = o.Key()
	}

	indexEntry, err := NewIndexEntry(cfg, o)
	if err != nil {
		return isShasums
	}

	v.AddBuild(indexEntry)

	return true
}

func NewVersionIndex(cfg IndexConfig, objectTree *ObjectTree) *VersionIndex {
	versionIndex := &VersionIndex{
		Name:    objectTree.ParentName(),
		Version: objectTree.DirName,
		Builds:  make([]*IndexEntry, 0),
	}

	unmatched := 0

	for _, v := range objectTree.Objects {
		if !versionIndex.addObject(cfg, v) {
			unmatched++
		}
	}

	if unmatched > 0 {
		log.Printf("%q: %d object(s) matched no release key pattern and are not in the index", objectTree.FullPath, unmatched) //nolint:gosec // %q escapes control characters in the S3-derived path
	}

	return versionIndex
}

type IndexEntry struct {
	Arch     string `json:"arch,omitempty"` // Dioad/Arch
	Filename string `json:"filename,omitempty"`
	Name     string `json:"name,omitempty"` // Dioad/Project
	Os       string `json:"os,omitempty"`   // Dioad/OS
	Size     int64  `json:"size,omitempty"` // bytes
	URL      string `json:"url,omitempty"`
	Version  string `json:"version,omitempty"` // Dioad/Version
}

func NewIndexEntry(cfg IndexConfig, o Object) (*IndexEntry, error) {
	releaseDetails, err := cfg.KeyExtractions.ExtractReleaseDetails(o.Key())
	if err != nil {
		return nil, err
	}

	if _, exists := releaseDetails["Version"]; !exists {
		return nil, errors.New("failed to extract version")
	}

	entry := &IndexEntry{
		Arch:     releaseDetails["Arch"],
		Filename: o.BaseName(),
		Name:     releaseDetails["Product"],
		Os:       releaseDetails["OS"],
		Size:     o.Size(),
		URL:      filepath.Join("/", o.Key()),
		Version:  releaseDetails["Version"],
	}

	return entry, nil
}

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/coreos/go-semver/semver"
)

var (
	DefaultReleaseInfoKeyExtractor = ReleaseDetailsKeyExtractor(`(?P<Prefix>.*?/)?(?P<Product>[^/]+)/(?P<Version>[^/]+)/(?P<PackageName>[^_]+)(_|_(?P<Extra>.*?)_)(?P<OS>[^_\d]+)_(?P<Arch>[^\.]+)\.(?P<ArchiveType>[^/]+)$`)
)

type ReleaseDetailsKeyExtractor string

func extractMetadataFromKey(regExp, key string) (map[string]string, error) {
	re, err := regexp.Compile(regExp)
	if err != nil {
		return nil, err
	}

	if !re.Match([]byte(key)) {
		return nil, fmt.Errorf("failed to match")
	}

	match := re.FindStringSubmatch(key)
	results := map[string]string{}
	for i, name := range match {
		if re.SubexpNames()[i] == "" {
			continue
		}
		results[re.SubexpNames()[i]] = name
	}

	return results, nil
}

func (k ReleaseDetailsKeyExtractor) ExtractReleaseDetails(key string) (map[string]string, error) {
	results, err := extractMetadataFromKey(string(k), key)
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
	return nil, fmt.Errorf("failed to match")
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

	// TODO: Latest Releases List of latest versions for each major version
	// LatestReleases map[string]*VersionIndex   `json:"releases"`

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
	}
}

func NewProductIndex(name string) *ProductIndex {
	return &ProductIndex{
		Name:     name,
		Versions: make(map[string]*VersionIndex),
		versions: make([]*semver.Version, 0),
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

func NewVersionIndex(cfg IndexConfig, objectTree *ObjectTree) *VersionIndex {
	versionIndex := &VersionIndex{
		Name:    objectTree.ParentName(),
		Version: objectTree.DirName,
		Builds:  make([]*IndexEntry, 0),
	}

	for _, v := range objectTree.Objects {
		indexEntry, err := NewIndexEntry(cfg, v)
		if err == nil {
			versionIndex.AddBuild(indexEntry)
		}

		if strings.HasSuffix(v.Key(), "_SHA256SUMS") {
			versionIndex.Shasums = v.Key()
		}
	}
	return versionIndex
}

type IndexEntry struct {
	Arch     string `json:"arch,omitempty"` // Dioad/Arch
	Filename string `json:"filename,omitempty"`
	Name     string `json:"name,omitempty"` // Dioad/Project
	Os       string `json:"os,omitempty"`   // Dioad/OS
	Url      string `json:"url,omitempty"`
	Version  string `json:"version,omitempty"` // Dioad/Version
}

func NewIndexEntry(cfg IndexConfig, o Object) (*IndexEntry, error) {
	releaseDetails, err := cfg.KeyExtractions.ExtractReleaseDetails(o.Key())
	if err != nil {
		return nil, err
	}

	if _, exists := releaseDetails["Version"]; !exists {
		return nil, fmt.Errorf("failed to extract version")
	}

	entry := &IndexEntry{
		Arch:     releaseDetails["Arch"],
		Filename: o.BaseName(),
		Name:     releaseDetails["Product"],
		Os:       releaseDetails["OS"],
		Url:      filepath.Join("/", o.Key()),
		Version:  releaseDetails["Version"],
	}

	return entry, nil
}

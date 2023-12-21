package main

import (
	"path/filepath"
	"strings"

	"github.com/coreos/go-semver/semver"
)

type IndexConfig struct {
	ArchitectureTagName string // "Dioad/Arch"
	ProductTagName      string // "Dioad/Project"
	OSTagName           string // "Dioad/OS"
	VersionTagName      string // "Dioad/Version"
}

type ArchiveIndex struct {
	Product map[string]*ProductIndex `json:"product,omitempty"`
}

func NewArchiveIndex() *ArchiveIndex {
	return &ArchiveIndex{
		Product: make(map[string]*ProductIndex),
	}
}

func (a *ArchiveIndex) AddProduct(p *ProductIndex) {
	a.Product[p.Name] = p
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
}

func (v *VersionIndex) AddBuild(build *IndexEntry) {
	v.Builds = append(v.Builds, build)
}

type IndexEntry struct {
	Arch     string `json:"arch,omitempty"` // Dioad/Arch
	Filename string `json:"filename,omitempty"`
	Name     string `json:"name,omitempty"` // Dioad/Project
	Os       string `json:"os,omitempty"`   // Dioad/OS
	Url      string `json:"url,omitempty"`
	Version  string `json:"version,omitempty"` // Dioad/Version
}

func NewIndexEntry(cfg IndexConfig, o Object) *IndexEntry {
	if _, exists := o.Tags()[cfg.VersionTagName]; !exists {
		return nil
	}

	return &IndexEntry{
		Arch:     o.Tags()[cfg.ArchitectureTagName],
		Filename: o.BaseName(),
		Name:     o.Tags()[cfg.ProductTagName],
		Os:       o.Tags()[cfg.OSTagName],
		Url:      filepath.Join("/", o.Key()),
		Version:  o.Tags()[cfg.VersionTagName],
	}
}

func NewVersionIndex(cfg IndexConfig, objectTree *ObjectTree) *VersionIndex {
	versionIndex := &VersionIndex{
		Name:    objectTree.ParentName(),
		Version: objectTree.DirName,
		Builds:  make([]*IndexEntry, 0),
	}

	for _, v := range objectTree.Objects {
		indexEntry := NewIndexEntry(cfg, v)
		if indexEntry != nil {
			versionIndex.AddBuild(indexEntry)
		}

		if strings.HasSuffix(v.Key(), "_SHA256SUMS") {
			versionIndex.Shasums = v.Key()
		}
	}
	return versionIndex
}

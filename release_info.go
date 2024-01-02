package main

func IsVersionTree(t *ObjectTree) bool {
	return t.DirName == "build" || IsVersionLabel(t.DirName)
}

func IsArchiveTree(t *ObjectTree) bool {
	for _, v := range t.Children {
		if IsProductTree(v) {
			return true
		}
	}
	return false
}

func IsProductTree(t *ObjectTree) bool {
	for _, v := range t.Children {
		if IsVersionTree(v) {
			return true
		}
	}
	return false
}

func NewArchiveIndexForObjectTree(cfg IndexConfig, t *ObjectTree) *ArchiveIndex {
	if !IsArchiveTree(t) {
		return nil
	}

	archiveIndex := NewArchiveIndex()

	for _, v := range t.Children {
		if IsProductTree(v) {
			productIndex := NewProductIndexForObjectTree(cfg, v)
			archiveIndex.AddProduct(productIndex)
		}
	}
	return archiveIndex
}

func NewProductIndexForObjectTree(cfg IndexConfig, t *ObjectTree) *ProductIndex {
	if !IsProductTree(t) {
		return nil
	}
	productIndex := NewProductIndex(t.DirName)

	for _, v := range t.Children {
		if IsVersionTree(v) {
			versionIndex := NewVersionIndexForObjectTree(cfg, v)
			productIndex.AddVersion(versionIndex)
		}
	}

	return productIndex
}

func NewVersionIndexForObjectTree(cfg IndexConfig, t *ObjectTree) *VersionIndex {
	if !IsVersionTree(t) {
		return nil
	}

	versionIndex := NewVersionIndex(cfg, t)

	return versionIndex
}

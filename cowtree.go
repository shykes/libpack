package libpack

import (
	"os"
)

type CowTree struct {
	changes Tree
	orig    Tree
}

func NewCowTree(orig Tree, changes Tree) *CowTree {
	if orig == nil {
		orig = make(Tree)
	}
	return &CowTree{
		changes: changes,
		orig:    orig,
	}
}

func (tree *CowTree) GetBlob(key string) (string, error) {
	changedVal, err := tree.changes.GetBlob(key)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err == nil {
		return changedVal, nil
	}
	// The value does not exist in the changes tree
	return tree.orig.GetBlob(key)
}

func (tree *CowTree) SubTree(key string, create bool) (*CowTree, error) {
	orig, err := tree.orig.SubTree(key, false)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	changes, err := tree.changes.SubTree(key, false)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return NewCowTree(orig, changes), nil
}

func (tree *CowTree) SetBlob(key, val string) error {
	return tree.changes.SetBlob(key, val)
}

func (tree *CowTree) List(key string) ([]string, error) {
	orig, err := tree.orig.List(key)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	changes, err := tree.changes.List(key)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	union := make(map[string]struct{})
	for _, k := range orig {
		union[k] = struct{}{}
	}
	for _, k := range changes {
		union[k] = struct{}{}
	}
	result := make([]string, 0, len(union))
	for k := range union {
		result = append(result, k)
	}
	return result, nil
}

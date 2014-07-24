package libpack

import (
	git "github.com/libgit2/git2go"
)

type GitTree struct {
	*git.Tree
}

func NewGitTree(tree *git.Tree) *GitTree {
	return &GitTree{Tree: tree}
}

func (tree *GitTree) GetBlob(key string) (string, error) {
	entry, err := tree.EntryByPath(key)
	if err != nil {
		return "", err
	}
	blob, err := lookupBlob(tree.Owner(), entry.Id)
	if err != nil {
		return "", err
	}
	return string(blob.Contents()), nil
}

func (tree *GitTree) SubTree(key string, create bool) (*GitTree, error) {
	// NOTE: `create` is ignored in this implementation because git trees
	// are immutable, so adding a subtree isn't possible.
	entry, err := tree.EntryByPath(key)
	if err != nil {
		return nil, err
	}
	t, err := lookupTree(tree.Owner(), entry.Id)
	if err != nil {
		return nil, err
	}
	return NewGitTree(t), nil
}

func (tree *GitTree) List(key string) ([]string, error) {
	var (
		i     uint64
		count uint64 = tree.EntryCount()
	)
	entries := make([]string, 0, count)
	for i = 0; i < count; i++ {
		entries = append(entries, tree.EntryByIndex(i).Name)
	}
	return entries, nil
}

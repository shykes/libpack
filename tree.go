package libpack

import (
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	git "github.com/libgit2/git2go"
)

// A Tree is an in-memory representation of a Git Tree object [1].
// It provides a familiar interface for constructing trees one
// entry at a time, like filesystem trees, then storing them into
// a git repository as immutable objects.
//
// [1] http://git-scm.com/book/en/Git-Internals-Git-Objects#Tree-Objects
type Tree map[string]interface{}

// FIXME: suport for whiteouts

func (tree Tree) Commit(repo, branch string) (string, error) {
	r, err := git.InitRepository(repo, true)
	if err != nil {
		return "", err
	}
	gt, err := tree.store(r)
	if err != nil {
		return "", err
	}
	var parents []*git.Commit
	if parentRef, err := r.LookupReference(branch); err == nil {
		// If the reference exists, use it as a parent,
		// otherwise create an initial commit.
		if parent, err := lookupCommit(r, parentRef.Target()); err != nil {
			return "", err
		} else {
			parents = append(parents, parent)
		}
	}
	commitId, err := r.CreateCommit(
		branch, // ref to update
		&git.Signature{"libpack", "libpack", time.Now()}, // author
		&git.Signature{"libpack", "libpack", time.Now()}, // committer
		"generated by libpack.Tree.Commit",               // message
		gt,         // git tree to commit
		parents..., // parent commit (0 or 1)
	)
	if err != nil {
		return "", err
	}
	return commitId.String(), nil
}

// Store writes the state of the in-memory tree to a new immutable tree object
// in the git repository at path `repo`, and returns its hash.
// Git computes tree hashes in a deterministic way, so if an identical tree already
// exists in the repo, its hash will be returned.
func (tree Tree) Store(repo string) (hash string, err error) {
	defer func() {
		if err != nil {
			fmt.Printf("[%p] Stored at %s\n", tree, hash)
		}
	}()
	r, err := git.InitRepository(repo, true)
	if err != nil {
		return "", err
	}
	gt, err := tree.store(r, nil)
	if err != nil {
		return "", err
	}
	return gt.Id().String(), nil
}

func (tree Tree) store(r *git.Repository, base *git.Tree) (*git.Tree, error) {
	var tb *git.TreeBuilder
	if base == nil {
		tb := r.TreeBuilder()
	} else {
		tb, err := r.TreeBuilderFromTree(base)
		if err != nil {
			return nil, err
		}
	}
	defer tb.Free()
	blobs := make(map[string]string)
	subtrees := make(map[string]Tree)
	tree.Walk(1,
		func(k string, subtree Tree) {
			subtrees[k] = subtree
		},
		func(k string, blob string) {
			blobs[k] = blob
		},
	)
	for prefix, subtree := range subtrees {
		fmt.Printf("[%p] Recursively storing sub-tree %s (%p)\n", tree, prefix, subtree)
		// Store the subtree
		gsubtree, err := subtree.store(r, nil)
		if err != nil {
			return nil, err
		}
		fmt.Printf("[%p]    -> %s tree stored at %s\n", tree, prefix, gsubtree.Id().String())
		// Add the subtree at `prefix/` in the current tree
		if err := tb.Insert(prefix, gsubtree.Id(), 040000); err != nil {
			return nil, err
		}
	}
	for key, hash := range blobs {
		fmt.Printf("[%p] Storing blob %s at %s\n", tree, hash, key)
		id, err := git.NewOid(hash)
		if err != nil {
			return nil, err
		}
		if err := tb.Insert(key, id, 0100644); err != nil {
			return nil, err
		}
	}
	treeId, err := tb.Write()
	if err != nil {
		return nil, err
	}
	return lookupTree(r, treeId)
}

// Pretty writes a human-readable description of the tree's contents
// to out.
func (tree Tree) Pretty(out io.Writer) {
	tree.Walk(0,
		func(k string, v Tree) {
			fmt.Fprintf(out, "[TREE] %40.40s %s\n", "", k)
		},
		func(k, v string) {
			fmt.Fprintf(out, "[BLOB] %s %s\n", v, k)
		},
	)
}

func (tree Tree) Walk(depth int, onTree func(string, Tree), onString func(string, string)) {
	for k, v := range tree {
		vString, isString := v.(string)
		if isString && onString != nil {
			onString(k, vString)
			continue
		}
		vTree, isTree := v.(Tree)
		if isTree && onTree != nil {
			onTree(k, vTree)
			if depth == 1 {
				continue
			}
			newDepth := depth - 1
			if newDepth < 0 {
				newDepth = 0
			}
			vTree.Walk(
				newDepth,
				func(subkey string, subtree Tree) {
					onTree(path.Join(k, subkey), subtree)
				},
				func(subkey string, subval string) {
					onString(path.Join(k, subkey), subval)
				},
			)
			continue
		}
	}
}

func (tree Tree) GetBlob(key string) (string, error) {
	base, leaf := path.Split(path.Clean(key))
	if leaf == "" {
		return "", fmt.Errorf("invalid path")
	}
	subtree, err := tree.SubTree(base, false)
	if err != nil {
		return "", err
	}
	val, exists := subtree[leaf]
	if !exists {
		return "", os.ErrNotExist
	}
	valString, isString := val.(string)
	if !isString {
		return "", fmt.Errorf("not a blob: %s", key)
	}
	return valString, nil
}

func (tree Tree) SubTree(key string, create bool) (t Tree, err error) {
	parts := pathParts(key)
	if len(parts) == 0 {
		return tree, nil
	}
	cursor := tree
	for _, part := range parts {
		val, exists := cursor[part]
		valTree, isTree := val.(Tree)
		// For each path component, 1 of the following is true:
		// 1. it doesn't exist
		// 2. it's a blob
		// 3. it's a subtree
		if !exists || !isTree {
			// If this component (1) doesn't exist or (2) it's a blob,
			// we must create a new subtree. This will overwrite the blob
			// if it exists.
			if !create {
				return nil, os.ErrNotExist
			}
			subtree := make(Tree)
			cursor[part] = subtree
			cursor = subtree
		} else {
			// If this path component is a tree, keep going
			cursor = valTree
		}
	}
	return cursor, nil
}

func (tree Tree) List(key string) ([]string, error) {
	dir, err := tree.SubTree(key, false)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(dir))
	for _, k := range keys {
		keys = append(keys, k)
	}
	return keys, nil
}

func (tree Tree) SetBlob(key, val string) error {
	var dir Tree
	base, leaf := path.Split(key)
	if base == "" {
		dir = tree
	} else {
		var err error
		dir, err = tree.SubTree(base, true)
		if err != nil {
			return err
		}
	}
	dir[leaf] = val
	return nil
}

func pathParts(p string) (parts []string) {
	p = path.Clean(p)
	// path.Clean("") returns "."
	if p == "." || p == "/" {
		return []string{}
	}
	p = strings.TrimLeft(p, "/")
	return strings.Split(p, "/")
}

func (tree Tree) Update(key string, val interface{}) error {
	key = path.Clean(key)
	key = strings.TrimLeft(key, "/") // Remove trailing slashes
	base, leaf := path.Split(key)
	if base == "" {
		// If val is a string, set it and we're done.
		// Any old value is overwritten.
		if valString, ok := val.(string); ok {
			tree[leaf] = valString
			return nil
		}
		// If val is not a string, it must be a subtree.
		// Return an error if it's any other type than Tree.
		valTree, ok := val.(Tree)
		if !ok {
			return fmt.Errorf("value must be a string or subtree")
		}
		// If that subtree already exists, merge the new one in.
		if old, exists := tree[leaf]; exists {
			oldTree, isTree := old.(Tree)
			if !isTree {
				return fmt.Errorf("key %s has existing value of unexpected type: %#v", key, old)
			}
			for k, v := range valTree {
				oldTree.Update(k, v)
			}
			return nil
		}
		tree[leaf] = val
		return nil
	}
	subtree := make(Tree)
	subtree[leaf] = val
	return tree.Update(base, subtree)
}
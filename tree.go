package libpack

import (
	"fmt"
	"io"
	"io/ioutil"
	"path"
	"strings"
)

// A Tree is an in-memory representation of a Git Tree object [1].
// It provides a familiar interface for constructing trees one
// entry at a time, like filesystem trees, then storing them into
// a git repository as immutable objects.
//
// [1] http://git-scm.com/book/en/Git-Internals-Git-Objects#Tree-Objects
type Tree map[string]interface{}

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
	// Initialize new index file
	tmp, err := ioutil.TempDir("", "tmpidx")
	if err != nil {
		return "", err
	}
	idx := path.Join(tmp, "idx")
	fmt.Printf("[%p] index file is at %s\n", tree, idx)
	// defer os.RemoveAll(idx)
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
		subtreehash, err := subtree.Store(repo)
		if err != nil {
			return "", err
		}
		fmt.Printf("[%p]    -> %s tree stored at %s\n", tree, prefix, subtreehash)
		// Add the subtree at `prefix/` in the current tree
		if err := gitReadTree(repo, idx, prefix, subtreehash); err != nil {
			return "", err
		}
	}
	for key, hash := range blobs {
		fmt.Printf("[%p] Storing blob %s at %s\n", tree, hash, key)
		if _, err := Git(repo, idx, "", nil, "update-index", "--add", "--cacheinfo", "100644", hash, key); err != nil {
			return "", err
		}
	}
	return gitWriteTree(repo, idx)
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

func (tree Tree) Update(key string, val interface{}) error {
	key = path.Clean(key)
	key = strings.TrimLeft(key, "/") // Remove trailing slashes
	base, leaf := path.Split(key)
	if base == "" {
		if valString, ok := val.(string); ok {
			tree[leaf] = valString
			return nil
		}
		valTree, ok := val.(Tree)
		if !ok {
			return fmt.Errorf("value must be a string or subtree")
		}
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

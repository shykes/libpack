package libpack

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"

	git "github.com/libgit2/git2go"

	"github.com/dotcloud/docker/vendor/src/code.google.com/p/go/src/pkg/archive/tar"
)

const (
	MetaTree = "_fs_meta"
	DataTree = "_fs_data"
)

func Git(repo, idx, worktree string, stdin io.Reader, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"--git-dir", repo}, args...)...)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	cmd.Stderr = os.Stderr
	if idx != "" {
		cmd.Env = append(cmd.Env, "GIT_INDEX_FILE="+idx)
	}
	if worktree != "" {
		cmd.Env = append(cmd.Env, "GIT_WORK_TREE="+worktree)
	}
	fmt.Printf("# %s %s\n", strings.Join(cmd.Env, " "), strings.Join(cmd.Args, " "))
	out, err := cmd.Output()
	return string(out), err
}

func gitHashObject(repo string, src io.Reader) (string, error) {
	out, err := Git(repo, "", "", src, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", fmt.Errorf("git hash-object: %v", err)
	}
	return strings.Trim(string(out), " \t\r\n"), nil
}

func gitWriteTree(repo, idx string) (string, error) {
	out, err := Git(repo, idx, "", nil, "write-tree")
	if err != nil {
		return "", fmt.Errorf("git write-tree: %v", err)
	}
	return strings.Trim(string(out), " \t\r\n"), nil
}

// gitReadTree calls 'git read-tree' with the following settings:
//	repo is the path to a git repo (bare)
//	idx is the path to the git index file to update
//	hash is the hash of the tree object to add
//	prefix is the prefix at which the tree should be added to the index file
func gitReadTree(repo, idx, prefix, hash string) error {
	worktree, err := ioutil.TempDir("", "tmpwd")
	if err != nil {
		return err
	}
	defer os.RemoveAll(worktree)
	if _, err := Git(repo, idx, worktree, nil, "read-tree", "--prefix", prefix, hash); err != nil {
		return fmt.Errorf("git-read-tree: %v", err)
	}
	return nil
}

// gitInit intializes a bare git repository at the path repo
func gitInit(repo string) error {
	_, err := Git(repo, "", "", nil, "init", "--bare", repo)
	if err != nil {
		return fmt.Errorf("git init: %v", err)
	}
	return nil
}

func lookupTree(repo *git.Repository, id *git.Oid) (*git.Tree, error) {
	obj, err := repo.Lookup(id)
	if err != nil {
		return nil, err
	}
	if tree, ok := obj.(*git.Tree); ok {
		return tree, nil
	}
	return nil, fmt.Errorf("hash %v exist but is not a tree", id)
}

func lookupBlob(repo *git.Repository, id *git.Oid) (*git.Blob, error) {
	obj, err := repo.Lookup(id)
	if err != nil {
		return nil, err
	}
	if blob, ok := obj.(*git.Blob); ok {
		return blob, nil
	}
	return nil, fmt.Errorf("hash %v exist but is not a blob", id)
}

func lookupSubtree(repo *git.Repository, tree *git.Tree, name string) (*git.Tree, error) {
	entry, err := tree.EntryByPath(name)
	if err != nil {
		return nil, err
	}
	return lookupTree(repo, entry.Id)
}

func lookupMetadata(repo *git.Repository, tree *git.Tree, name string) (*tar.Header, error) {
	entry, err := tree.EntryByPath(metaPath(name))
	if err != nil {
		return nil, err
	}
	blob, err := lookupBlob(repo, entry.Id)
	if err != nil {
		return nil, err
	}
	defer blob.Free()
	tr := tar.NewReader(bytes.NewReader(blob.Contents()))
	hdr, err := tr.Next()
	if err != nil {
		return nil, err
	}
	return hdr, nil
}

// Git2tar looks for a git tree object at `hash` in a git repository at the path
// `repo`, then extracts it as a tar stream written to `dst`.
// The tree is not buffered on disk or in memory before being streamed.
func Git2tar(repo, hash string, dst io.Writer) error {
	tw := tar.NewWriter(dst)
	r, err := git.InitRepository(repo, true)
	if err != nil {
		return err
	}
	defer r.Free()
	// Lookup the tree object at `hash` in `repo`
	treeId, err := git.NewOid(hash)
	if err != nil {
		return err
	}
	tree, err := lookupTree(r, treeId)
	if err != nil {
		return err
	}
	defer tree.Free()
	metaTree, err := lookupSubtree(r, tree, MetaTree)
	if err != nil {
		return err
	}
	defer metaTree.Free()
	dataTree, err := lookupSubtree(r, tree, DataTree)
	if err != nil {
		return err
	}
	// Walk the data tree
	var walkErr error
	if err := dataTree.Walk(func(name string, entry *git.TreeEntry) int {
		// FIXME: is it normal that Walk() passes an empty name?
		// If so, what's the correct way to handle it?
		// For now we just skip it.
		if name == "" {
			return 0
		}
		// For each element (blob or subtree) look up the corresponding tar header
		// from the meta tree
		hdr, err := lookupMetadata(r, tree, name)
		if err != nil {
			walkErr = fmt.Errorf("metadata lookup for '%s': %v", name, err)
			return -1
		}
		// Write the reconstituted tar header+content
		if err := tw.WriteHeader(hdr); err != nil {
			walkErr = err
			return -1
		}
		if entry.Type == git.ObjectBlob {
			blob, err := lookupBlob(r, entry.Id)
			if err != nil {
				walkErr = err
				return -1
			}
			if _, err := tw.Write(blob.Contents()); err != nil {
				walkErr = err
				return -1
			}
		}
		return 0
	}); err != nil {
		if walkErr != nil {
			return walkErr
		}
		return err
	}
	return nil
}

func metaPath(name string) string {
	// FIXME: this doesn't seem to yield the expected result.
	return path.Join("_fs_meta", fmt.Sprintf("%0x", sha1.Sum([]byte(name))))
}

// Tar2git decodes a tar stream from src, then encodes it into a new git commit
// such that the full tar stream can be reconsistuted from the git data alone.
// It retusn hash of the git commit, or an error if any.
func Tar2git(src io.Reader, repo string) (hash string, err error) {
	if err := gitInit(repo); err != nil {
		return "", err
	}

	tree := make(Tree)
	tr := tar.NewReader(src)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		fmt.Printf("[META] %s\n", hdr.Name)
		metaBlob, err := headerReader(hdr)
		if err != nil {
			return "", err
		}
		metaHash, err := gitHashObject(repo, metaBlob)
		if err != nil {
			return "", err
		}
		metaDst := metaPath(hdr.Name)
		fmt.Printf("    ---> storing metadata in %s\n", metaDst)
		if err := tree.Update(metaDst, metaHash); err != nil {
			return "", err
		}
		// FIXME: git can carry symlinks as well
		if hdr.Typeflag == tar.TypeReg {
			fmt.Printf("[DATA] %s %d bytes\n", hdr.Name, hdr.Size)
			dataHash, err := gitHashObject(repo, tr)
			if err != nil {
				return "", err
			}
			dataDst := path.Join("_fs_data", hdr.Name)
			if err := tree.Update(dataDst, dataHash); err != nil {
				return "", err
			}
		}
	}
	tree.Pretty(os.Stdout)
	return tree.Store(repo)
}

type Tree map[string]interface{}

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

func headerReader(hdr *tar.Header) (io.Reader, error) {
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	defer w.Close()
	if err := w.WriteHeader(hdr); err != nil {
		return nil, err
	}
	return &buf, nil
}

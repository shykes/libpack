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
	"sync"

	git "github.com/libgit2/git2go"
	"github.com/dotcloud/docker/archive"

	"github.com/dotcloud/docker/vendor/src/code.google.com/p/go/src/pkg/archive/tar"
)

const (
	MetaTree = "_fs_meta"
	DataTree = "_fs_data"
)

func Pack(repo, dir string) (hash string, err error) {
	a, err := archive.TarWithOptions(dir, &archive.TarOptions{Excludes: []string{".git"}})
	if err != nil {
		return "", err
	}
	return Tar2git(a, repo)
}

func Unpack(repo, dir, hash string) error {
	r, w := io.Pipe()
	var (
		inErr error
		outErr error
	)
	var tasks sync.WaitGroup
	tasks.Add(2)
	go func() {
		defer tasks.Done()
		inErr = Git2tar(repo, hash, os.Stdout)
		w.Close()
	}()
	go func() {
		defer tasks.Done()
		outErr = archive.Untar(r, dir, &archive.TarOptions{})
	}()
	tasks.Wait()
	if inErr != nil {
		return fmt.Errorf("git2tar: %v", inErr)
	}
	if outErr != nil {
		return fmt.Errorf("untar: %v", outErr)
	}
	return nil
}

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

// lookupTree looks up an object at hash `id` in `repo`, and returns
// it as a git tree. If the object is not a tree, an error is returned.
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

// lookupBlob looks up an object at hash `id` in `repo`, and returns
// it as a git blob. If the object is not a blob, an error is returned.
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

// lookupTree looks up the entry `name` under the git tree `Tree` in
// repository `repo`. If `name` includes slashes, it is interpreted as
// a path in the tree.
// If the entry exists, the object it points to is is returned, converted
// to a git tree (ie a sub-tree).
// If the entry does not point to a tree (the other option being a blob),
// an error is returned.
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

// metaPath computes a path at which the metadata can be stored for a given path.
// For example if `name` is "/etc/resolv.conf", the corresponding metapath is 
// "_fs_meta/194c1cbe5a8cfcb85c6a46b936da12ffdc32f90f"
// This path will be used to store and retrieve the tar header encoding the metadata
// for the corresponding file.
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

func headerReader(hdr *tar.Header) (io.Reader, error) {
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	defer w.Close()
	if err := w.WriteHeader(hdr); err != nil {
		return nil, err
	}
	return &buf, nil
}

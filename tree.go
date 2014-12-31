package libpack

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"

	git "github.com/libgit2/git2go"
)

// A Tree is a simple data structure that is API-compatible
// with git tree objects.
// It is essentially an immutable directory in a content-addressable
// filesystem made of directories (trees) and files (blobs).
type Tree struct {
	*git.Tree
	r *Repository
}

// A value is either a Tree or a string.
// Values can be stored in trees.
type Value interface {
	IfString(func(string)) error
	IfTree(func(*Tree)) error
}

// Hash returns the immutable, globally unique SHA hash
// of the tree.
func (t *Tree) Hash() string {
	return t.Id().String()
}

// Repository returns the Repository backing this tree.
func (t *Tree) Repo() *Repository {
	return t.r
}

func (t *Tree) Get(key string) (string, error) {
	if t == nil {
		return "", os.ErrNotExist
	}
	key = TreePath(key)
	e, err := t.EntryByPath(key)
	if err != nil {
		return "", err
	}
	blob, err := lookupBlob(t.r.gr, e.Id)
	if err != nil {
		return "", err
	}
	defer blob.Free()
	return string(blob.Contents()), nil
}

func (t *Tree) Set(key, val string) (*Tree, error) {
	// FIXME: libgit2 crashes if value is empty.
	// Work around this by shelling out to git.
	id, err := t.r.gr.CreateBlobFromBuffer([]byte(val))
	if err != nil {
		return nil, err
	}
	return t.addGitObj(key, id.String(), true)
}

// SetStream writes the data from `src` to a new Git blob,
// and updates the uncommitted tree to point to that blob as `key`.
func (t *Tree) SetStream(key string, src io.Reader) (*Tree, error) {
	// FIXME: instead of buffering the entire value, use
	// libgit2 CreateBlobFromChunks to stream the data straight
	// into git.
	buf := new(bytes.Buffer)
	_, err := io.Copy(buf, src)
	if err != nil {
		return nil, err
	}
	return t.Set(key, buf.String())
}

func (t *Tree) Mkdir(key string) (*Tree, error) {
	// FIXME: specify the behavior of Mkdir when the key already exists.
	return t.Pipeline().AddQuery(key, t.Pipeline().Empty(), true).Run()
}

func (t *Tree) Empty() (*Tree, error) {
	return t.r.EmptyTree()
}

func (t *Tree) Delete(key string) (*Tree, error) {
	gt, err := treeDel(t.r.gr, t.Tree, key)
	if err != nil {
		return nil, err
	}
	return &Tree{
		Tree: gt,
		r:    t.r,
	}, nil
}

func (t *Tree) Diff(other *Tree) (added, removed *Tree, err error) {
	// FIXME
	return nil, nil, fmt.Errorf("not implemented")
}

func (t *Tree) Free() {
	t.Tree.Free()
}

type WalkHandler func(string, Value) error

func (t *Tree) List(key string) ([]string, error) {
	subtree, err := t.Scope(key)
	if err != nil {
		return nil, err
	}
	defer subtree.Free()
	var (
		i     uint64
		count uint64 = subtree.EntryCount()
	)
	entries := make([]string, 0, count)
	for i = 0; i < count; i++ {
		entries = append(entries, subtree.Tree.EntryByIndex(i).Name)
	}
	return entries, nil
}

func (t *Tree) Walk(h WalkHandler) error {
	return treeWalk(t.r.gr, t.Tree, "/", func(k string, o git.Object) error {
		// FIXME: translate to higher-level handler
		return fmt.Errorf("not implemented")
	})
}

func (t *Tree) Add(key string, overlay *Tree, merge bool) (*Tree, error) {
	return t.addGitObj(key, overlay.Hash(), merge)
}

func (t *Tree) Subtract(key string, whiteout *Tree) (*Tree, error) {
	// FIXME
	return nil, fmt.Errorf("not implemented")
}

func (t *Tree) Scope(key string) (*Tree, error) {
	gt, err := treeScope(t.r.gr, t.Tree, key)
	if err != nil {
		return nil, err
	}
	return &Tree{
		Tree: gt,
		r:    t.r,
	}, nil
}

func (t *Tree) Dump(dst io.Writer) error {
	return treeDump(t.r.gr, t.Tree, "/", dst)
}

// Checkout populates the directory at dir with the contents of the tree.
//
// As a convenience, if dir is an empty string, a temporary directory
// is created and returned, and the caller is responsible for removing it.
//
// FIXME: this does not work properly at the moment.
//
func (t *Tree) Checkout(dir string) (checkoutDir string, err error) {
	// FIXME: Tree.Checkout does not work properly at the moment
	return "", fmt.Errorf("FIXME: known bug")

	// If the tree is empty, checkout will fail and there is
	// nothing to do anyway
	if t.EntryCount() == 0 {
		return "", nil
	}
	idx, err := ioutil.TempFile("", "libpack-index")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(idx.Name())
	readTree := exec.Command(
		"git",
		"--git-dir", t.r.gr.Path(),
		"--work-tree", dir,
		"read-tree", t.Tree.Id().String(),
	)
	readTree.Env = append(readTree.Env, "GIT_INDEX_FILE="+idx.Name())
	stderr := new(bytes.Buffer)
	readTree.Stderr = stderr
	if err := readTree.Run(); err != nil {
		return "", fmt.Errorf("%s", stderr.String())
	}
	checkoutIndex := exec.Command(
		"git",
		"--git-dir", t.r.gr.Path(),
		"--work-tree", dir,
		"checkout-index",
	)
	checkoutIndex.Env = append(checkoutIndex.Env, "GIT_INDEX_FILE="+idx.Name())
	stderr = new(bytes.Buffer)
	checkoutIndex.Stderr = stderr
	if err := checkoutIndex.Run(); err != nil {
		return "", fmt.Errorf("%s", stderr.String())
	}
	return "", nil
}

// ExecInCheckout checks out the committed contents of the database into a
// temporary directory, executes the specified command in a new subprocess
// with that directory as the working directory, then removes the directory.
//
// The standard input, output and error streams of the command are the same
// as the current process's.
func (t *Tree) ExecInCheckout(path string, args ...string) error {
	checkout, err := t.Checkout("")
	if err != nil {
		return fmt.Errorf("checkout: %v", err)
	}
	defer os.RemoveAll(checkout)
	cmd := exec.Command(path, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = checkout
	return cmd.Run()
}

func (t *Tree) Pipeline() *Pipeline {
	return NewPipeline(t.r).Add("/", t, false)
}

func (t *Tree) addGitObj(key string, hash string, merge bool) (*Tree, error) {
	valueId, err := git.NewOid(hash)
	if err != nil {
		return nil, err
	}
	gt, err := treeAdd(t.r.gr, t.Tree, key, valueId, merge)
	if err != nil {
		return nil, err
	}
	return &Tree{
		Tree: gt,
		r:    t.r,
	}, nil
}

func TreePath(p string) string {
	p = path.Clean(p)
	if p == "/" || p == "." {
		return "/"
	}
	// Remove leading / from the path
	// as libgit2.TreeEntryByPath does not accept it
	p = strings.TrimLeft(p, "/")
	return p
}

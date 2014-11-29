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

type Tree struct {
	*git.Tree
	r *git.Repository
}

type Value interface {
	IfString(func(string)) error
	IfTree(func(*Tree)) error
}

func treeFromGit(r *git.Repository, id *git.Oid) (*Tree, error) {
	gt, err := lookupTree(r, id)
	if err == nil {
		return &Tree{
			Tree: gt,
			r:    r,
		}, nil
	}
	gc, err := lookupCommit(r, id)
	if err == nil {
		gt, err := gc.Tree()
		if err != nil {
			return nil, err
		}
		return &Tree{
			Tree: gt,
			r:    r,
		}, nil
	}
	return nil, fmt.Errorf("not a valid tree or commit: %s", id)
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
	blob, err := lookupBlob(t.r, e.Id)
	if err != nil {
		return "", err
	}
	defer blob.Free()
	return string(blob.Contents()), nil
}

func (t *Tree) Set(key, val string) (*Tree, error) {
	// FIXME: libgit2 crashes if value is empty.
	// Work around this by shelling out to git.
	var (
		id  *git.Oid
		err error
	)
	if val == "" {
		out, err := exec.Command("git", "--git-dir", t.r.Path(), "hash-object", "-w", "--stdin").Output()
		if err != nil {
			return nil, fmt.Errorf("git hash-object: %v", err)
		}
		id, err = git.NewOid(strings.Trim(string(out), " \t\r\n"))
		if err != nil {
			return nil, fmt.Errorf("git newoid %v", err)
		}
	} else {
		id, err = t.r.CreateBlobFromBuffer([]byte(val))
		if err != nil {
			return nil, err
		}
	}
	return t.addGitObj(key, id, true)
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
	empty, err := emptyTree(t.r)
	if err != nil {
		return nil, err
	}
	return t.addGitObj(key, empty, true)
}

func (t *Tree) Empty() (*Tree, error) {
	id, err := emptyTree(t.r)
	if err != nil {
		return nil, err
	}
	gt, err := lookupTree(t.r, id)
	if err != nil {
		return nil, err
	}
	return &Tree{
		Tree: gt,
		r:    t.r,
	}, nil
}

func (t *Tree) Delete(key string) (*Tree, error) {
	gt, err := treeDel(t.r, t.Tree, key)
	if err != nil {
		return nil, err
	}
	return &Tree{
		Tree: gt,
		r:    t.r,
	}, nil
}

func (t *Tree) Diff(other Tree) (added, removed *Tree, err error) {
	// FIXME
	return nil, nil, fmt.Errorf("not implemented")
}

type WalkHandler func(string, Value) error

func (t *Tree) Walk(h WalkHandler) error {
	return treeWalk(t.r, t.Tree, "/", func(k string, o git.Object) error {
		// FIXME: translate to higher-level handler
		return fmt.Errorf("not implemented")
	})
}

func (t *Tree) Add(key string, overlay *Tree, merge bool) (*Tree, error) {
	return t.addGitObj(key, overlay.Tree.Id(), merge)
}

func (t *Tree) Subtract(key string, whiteout *Tree) (*Tree, error) {
	// FIXME
	return nil, fmt.Errorf("not implemented")
}

func (t *Tree) Scope(key string) (*Tree, error) {
	gt, err := treeScope(t.r, t.Tree, key)
	if err != nil {
		return nil, err
	}
	return &Tree{
		Tree: gt,
		r:    t.r,
	}, nil
}

func (t *Tree) Dump(dst io.Writer) error {
	return treeDump(t.r, t.Tree, "/", dst)
}

// Checkout populates the directory at dir with the contents of the tree.
//
// As a convenience, if dir is an empty string, a temporary directory
// is created and returned, and the caller is responsible for removing it.
//
// FIXME: this does not work properly at the moment.
//
func (t *Tree) Checkout(dir string) (checkoutDir string, err error) {
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
		"--git-dir", t.r.Path(),
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
		"--git-dir", t.r.Path(),
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

// FIXME: port pipeline to Tree

func (t *Tree) Pipeline() *Pipeline {
	return &Pipeline{
		op: OpNop,
	}
}

func (t *Tree) addGitObj(key string, valueId *git.Oid, merge bool) (*Tree, error) {
	gt, err := treeAdd(t.r, t.Tree, key, valueId, merge)
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

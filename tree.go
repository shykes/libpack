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

func TreeFromGit(r *git.Repository, id *git.Oid) (*Tree, error) {
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
	return TreeWalk(t.r, t.Tree, "/", func(k string, o git.Object) error {
		// FIXME: translate to higher-level handler
		return fmt.Errorf("not implemented")
	})
}

func (t *Tree) Add(key string, overlay *Tree, merge bool) (*Tree, error) {
	return t.addGitObj(key, overlay.Tree.Id(), merge)
}

func (t *Tree) Substract(key string, whiteout *Tree) (*Tree, error) {
	// FIXME
	return nil, fmt.Errorf("not implemented")
}

func (t *Tree) Scope(key string) (*Tree, error) {
	gt, err := TreeScope(t.r, t.Tree, key)
	if err != nil {
		return nil, err
	}
	return &Tree{
		Tree: gt,
		r:    t.r,
	}, nil
}

func (t *Tree) Dump(dst io.Writer) error {
	return TreeDump(t.r, t.Tree, "/", dst)
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

// Removes a key from the tree.
func treeDel(repo *git.Repository, tree *git.Tree, key string) (*git.Tree, error) {
	var err error

	key = TreePath(key)
	base, leaf := path.Split(key)

	if tree != nil {
		if tree, err = TreeScope(repo, tree, base); err != nil {
			return nil, err
		}
	}

	builder, err := repo.TreeBuilderFromTree(tree)
	if err != nil {
		return nil, err
	}

	if err := builder.Remove(leaf); err != nil {
		return nil, err
	}

	treeId, err := builder.Write()
	if err != nil {
		return nil, err
	}

	newTree, err := lookupTree(repo, treeId)
	if err != nil {
		return nil, err
	}

	return newTree, err
}

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

// treeAdd creates a new Git tree by adding a new object
// to it at the specified path.
// Intermediary subtrees are created as needed.
// If an object already exists at key or any intermediary path,
// it is overwritten.
//
// - If merge is true, new trees are merged into existing ones at the
// file granularity (similar to 'cp -R').
// - If it is false, existing trees are completely shadowed (similar to 'mount')
//
// Since git trees are immutable, base is not modified. The new
// tree is returned.
// If an error is encountered, intermediary objects may be left
// behind in the git repository. It is the caller's responsibility
// to perform garbage collection, if any.
// FIXME: manage garbage collection, or provide a list of created
// objects.
func treeAdd(repo *git.Repository, tree *git.Tree, key string, valueId *git.Oid, merge bool) (t *git.Tree, err error) {
	/*
	** // Primitive but convenient tracing for debugging recursive calls to treeAdd.
	** // Uncomment this block for debug output.
	**
	** var callString string
	** if tree != nil {
	** 		callString = fmt.Sprintf("   treeAdd %v:\t\t%s\t\t\t= %v", tree.Id(), key, valueId)
	** 	} else {
	** 		callString = fmt.Sprintf("   treeAdd %v:\t\t%s\t\t\t= %v", tree, key, valueId)
	** 	}
	** 	fmt.Printf("   %s\n", callString)
	** 	defer func() {
	** 		if t != nil {
	** 			fmt.Printf("-> %s => %v\n", callString, t.Id())
	** 		} else {
	** 			fmt.Printf("-> %s => %v\n", callString, err)
	** 		}
	** 	}()
	 */
	if valueId == nil {
		return tree, nil
	}
	key = TreePath(key)
	base, leaf := path.Split(key)
	o, err := repo.Lookup(valueId)
	if err != nil {
		return nil, err
	}
	var builder *git.TreeBuilder
	if tree == nil {
		builder, err = repo.TreeBuilder()
		if err != nil {
			return nil, err
		}
	} else {
		builder, err = repo.TreeBuilderFromTree(tree)
		if err != nil {
			return nil, err
		}
	}
	defer builder.Free()
	// The specified path has only 1 component (the "leaf")
	if base == "" || base == "/" {
		// If val is a string, set it and we're done.
		// Any old value is overwritten.
		if _, isBlob := o.(*git.Blob); isBlob {
			if err := builder.Insert(leaf, valueId, 0100644); err != nil {
				return nil, err
			}
			newTreeId, err := builder.Write()
			if err != nil {
				return nil, err
			}
			newTree, err := lookupTree(repo, newTreeId)
			if err != nil {
				return nil, err
			}
			return newTree, nil
		}
		// If val is not a string, it must be a subtree.
		// Return an error if it's any other type than Tree.
		oTree, ok := o.(*git.Tree)
		if !ok {
			return nil, fmt.Errorf("value must be a blob or subtree")
		}
		var subTree *git.Tree
		var oldSubTree *git.Tree
		if tree != nil {
			oldSubTree, err = TreeScope(repo, tree, leaf)
			// FIXME: distinguish "no such key" error (which
			// FIXME: distinguish a non-existing previous tree (continue with oldTree==nil)
			// from other errors (abort and return an error)
			if err == nil {
				defer oldSubTree.Free()
			}
		}
		// If that subtree already exists, merge the new one in.
		if merge && oldSubTree != nil {
			subTree = oldSubTree
			for i := uint64(0); i < oTree.EntryCount(); i++ {
				var err error
				e := oTree.EntryByIndex(i)
				subTree, err = treeAdd(repo, subTree, e.Name, e.Id, merge)
				if err != nil {
					return nil, err
				}
			}
		} else {
			subTree = oTree
		}
		// If the key is /, we're replacing the current tree
		if key == "/" {
			return subTree, nil
		}
		// Otherwise we're inserting into the current tree
		if err := builder.Insert(leaf, subTree.Id(), 040000); err != nil {
			return nil, err
		}
		newTreeId, err := builder.Write()
		if err != nil {
			return nil, err
		}
		newTree, err := lookupTree(repo, newTreeId)
		if err != nil {
			return nil, err
		}
		return newTree, nil
	}
	subtree, err := treeAdd(repo, nil, leaf, valueId, merge)
	if err != nil {
		return nil, err
	}
	return treeAdd(repo, tree, base, subtree.Id(), merge)
}

func TreeGet(r *git.Repository, t *git.Tree, key string) (string, error) {
	if t == nil {
		return "", os.ErrNotExist
	}
	key = TreePath(key)
	e, err := t.EntryByPath(key)
	if err != nil {
		return "", err
	}
	blob, err := lookupBlob(r, e.Id)
	if err != nil {
		return "", err
	}
	defer blob.Free()
	return string(blob.Contents()), nil

}

func TreeList(r *git.Repository, t *git.Tree, key string) ([]string, error) {
	if t == nil {
		return []string{}, nil
	}
	subtree, err := TreeScope(r, t, key)
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
		entries = append(entries, subtree.EntryByIndex(i).Name)
	}
	return entries, nil
}

func TreeWalk(r *git.Repository, t *git.Tree, key string, h func(string, git.Object) error) error {
	if t == nil {
		return fmt.Errorf("no tree to walk")
	}
	subtree, err := TreeScope(r, t, key)
	if err != nil {
		return err
	}
	var handlerErr error
	err = subtree.Walk(func(parent string, e *git.TreeEntry) int {
		obj, err := r.Lookup(e.Id)
		if err != nil {
			handlerErr = err
			return -1
		}
		if err := h(path.Join(parent, e.Name), obj); err != nil {
			handlerErr = err
			return -1
		}
		obj.Free()
		return 0
	})
	if handlerErr != nil {
		return handlerErr
	}
	if err != nil {
		return err
	}
	return nil
}

func TreeDump(r *git.Repository, t *git.Tree, key string, dst io.Writer) error {
	return TreeWalk(r, t, key, func(key string, obj git.Object) error {
		if _, isTree := obj.(*git.Tree); isTree {
			fmt.Fprintf(dst, "%s/\n", key)
		} else if blob, isBlob := obj.(*git.Blob); isBlob {
			fmt.Fprintf(dst, "%s = %s\n", key, blob.Contents())
		}
		return nil
	})
}

func TreeScope(repo *git.Repository, tree *git.Tree, name string) (*git.Tree, error) {
	if tree == nil {
		return nil, fmt.Errorf("tree undefined")
	}
	name = TreePath(name)
	if name == "/" {
		// Allocate a new Tree object so that the caller
		// can always call Free() on the result
		return lookupTree(repo, tree.Id())
	}
	entry, err := tree.EntryByPath(name)
	if err != nil {
		return nil, err
	}
	return lookupTree(repo, entry.Id)
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

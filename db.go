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
	"time"

	git "github.com/libgit2/git2go"
)

// DB is a simple git-backed database.
type DB struct {
	repo   *git.Repository
	commit *git.Commit
	ref    string
	scope  string
	tree   *git.Tree
	parent *DB
}

func (db *DB) Scope(scope string) *DB {
	// FIXME: do we risk duplicate db.repo.Free()?
	return &DB{
		repo:   db.repo,
		commit: db.commit,
		ref:    db.ref,
		scope:  scope, // If parent!=nil, scope is relative to parent
		tree:   db.tree,
		parent: db,
	}
}

// Init initializes a new git-backed database from the following
// elements:
// * A bare git repository at `repo`
// * A git reference name `ref` (for example "refs/heads/foo")
// * An optional scope to expose only a subset of the git tree (for example "/myapp/v1")
func Init(repo, ref string) (*DB, error) {
	r, err := git.InitRepository(repo, true)
	if err != nil {
		return nil, err
	}
	db, err := newRepo(r, ref)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func Open(repo, ref string) (*DB, error) {
	r, err := git.OpenRepository(repo)
	if err != nil {
		return nil, err
	}
	db, err := newRepo(r, ref)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func newRepo(repo *git.Repository, ref string) (*DB, error) {
	db := &DB{
		repo: repo,
		ref:  ref,
	}
	if err := db.Update(); err != nil {
		db.Free()
		return nil, err
	}
	return db, nil
}

// Free must be called to release resources when a database is no longer
// in use.
// This is required in addition to Golang garbage collection, because
// of the libgit2 C bindings.
func (db *DB) Free() {
	db.repo.Free()
	if db.commit != nil {
		db.commit.Free()
	}
}

// Head returns the id of the latest commit
func (db *DB) Head() *git.Oid {
	if db.commit != nil {
		return db.commit.Id()
	}
	return nil
}

func (db *DB) Latest() *git.Oid {
	if db.tree != nil {
		return db.tree.Id()
	}
	return nil
}

func (db *DB) Repo() *git.Repository {
	return db.repo
}

func (db *DB) Dump(dst io.Writer) error {
	return db.Walk("/", func(key string, obj git.Object) error {
		if _, isTree := obj.(*git.Tree); isTree {
			fmt.Fprintf(dst, "%s/\n", key)
		} else if blob, isBlob := obj.(*git.Blob); isBlob {
			fmt.Fprintf(dst, "%s = %s\n", key, blob.Contents())
		}
		return nil
	})
}

// AddDB copies the contents of src into db at prefix key.
// The resulting content is the union of the new tree and
// the pre-existing tree, if any.
// In case of a conflict, the content of the new tree wins.
// Conflicts are resolved at the file granularity (content is
// never merged).
func (db *DB) AddDB(key string, src *DB) error {
	// No tree to add, nothing to do
	if src.tree == nil {
		return nil
	}
	return db.Add(key, src.tree.Id())
}

func (db *DB) Add(key string, obj interface{}) error {
	if db.parent != nil {
		return db.parent.Add(path.Join(db.scope, key), obj)
	}
	newTree, err := NewPipeline(db.repo).Base(db.tree).Add(key, obj, true).Run()
	if err != nil {
		return err
	}
	db.tree = newTree
	return nil
}

func (db *DB) Walk(key string, h func(string, git.Object) error) error {
	if db.tree == nil {
		return fmt.Errorf("no tree to walk")
	}
	subtree, err := lookupSubtree(db.repo, db.tree, key)
	if err != nil {
		return err
	}
	var handlerErr error
	err = subtree.Walk(func(parent string, e *git.TreeEntry) int {
		obj, err := db.repo.Lookup(e.Id)
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

// Update looks up the value of the database's reference, and changes
// the memory representation accordingly.
// If the committed tree is changed, then uncommitted changes are lost.
func (db *DB) Update() error {
	tip, err := db.repo.LookupReference(db.ref)
	if err != nil {
		db.commit = nil
		return nil
	}
	// If we already have the latest commit, don't do anything
	if db.commit != nil && db.commit.Id().Equal(tip.Target()) {
		return nil
	}
	commit, err := db.lookupCommit(tip.Target())
	if err != nil {
		return err
	}
	if db.commit != nil {
		db.commit.Free()
	}
	db.commit = commit
	if db.tree != nil {
		db.tree.Free()
	}
	if commitTree, err := commit.Tree(); err != nil {
		return err
	} else {
		db.tree = commitTree
	}
	return nil
}

// Mkdir adds an empty subtree at key if it doesn't exist.
func (db *DB) Mkdir(key string) error {
	if db.parent != nil {
		return db.parent.Mkdir(path.Join(db.scope, key))
	}
	p := NewPipeline(db.repo)
	newTree, err := p.Base(db.tree).Mkdir(path.Join(db.scope, key)).Run()
	if err != nil {
		return err
	}
	db.tree = newTree
	return nil
}

// Get returns the value of the Git blob at path `key`.
// If there is no blob at the specified key, an error
// is returned.
func (db *DB) Get(key string) (string, error) {
	if db.parent != nil {
		return db.parent.Get(path.Join(db.scope, key))
	}
	if db.tree == nil {
		return "", os.ErrNotExist
	}
	key = TreePath(key)
	e, err := db.tree.EntryByPath(path.Join(db.scope, key))
	if err != nil {
		return "", err
	}
	blob, err := db.lookupBlob(e.Id)
	if err != nil {
		return "", err
	}
	defer blob.Free()
	return string(blob.Contents()), nil
}

// Set writes the specified value in a Git blob, and updates the
// uncommitted tree to point to that blob as `key`.
func (db *DB) Set(key, value string) error {
	if db.parent != nil {
		return db.parent.Set(path.Join(db.scope, key), value)
	}
	p := NewPipeline(db.repo)
	newTree, err := p.Base(db.tree).Set(path.Join(db.scope, key), value).Run()
	if err != nil {
		return err
	}
	db.tree = newTree
	return nil
}

// SetStream writes the data from `src` to a new Git blob,
// and updates the uncommitted tree to point to that blob as `key`.
func (db *DB) SetStream(key string, src io.Reader) error {
	// FIXME: instead of buffering the entire value, use
	// libgit2 CreateBlobFromChunks to stream the data straight
	// into git.
	buf := new(bytes.Buffer)
	_, err := io.Copy(buf, src)
	if err != nil {
		return err
	}
	return db.Set(key, buf.String())
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

// List returns a list of object names at the subtree `key`.
// If there is no subtree at `key`, an error is returned.
func (db *DB) List(key string) ([]string, error) {
	if db.tree == nil {
		return []string{}, nil
	}
	subtree, err := lookupSubtree(db.repo, db.tree, path.Join(db.scope, key))
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

// Commit atomically stores all database changes since the last commit
// into a new Git commit object, and updates the database's reference
// to point to that commit.
func (db *DB) Commit(msg string) error {
	if db.parent != nil {
		return db.parent.Commit(msg)
	}
	if db.tree == nil {
		// Nothing to commit
		return nil
	}
	// FIXME: the ref might have been changed by another
	// process. We must implement either 1) reliable locking
	// or 2) a solid merge resolution strategy.
	// For now we simply assume the ref has not changed.
	var parents []*git.Commit
	if db.commit != nil {
		commitTree, err := db.commit.Tree()
		if err != nil {
			return err
		}
		if commitTree.Id().Equal(db.tree.Id()) {
			// Nothing to commit
			return nil
		}
		parents = append(parents, db.commit)
	}
	commitId, err := db.repo.CreateCommit(
		db.ref,
		&git.Signature{"libpack", "libpack", time.Now()}, // author
		&git.Signature{"libpack", "libpack", time.Now()}, // committer
		msg,
		db.tree,    // git tree to commit
		parents..., // parent commit (0 or 1)
	)
	if err != nil {
		return err
	}
	commit, err := db.lookupCommit(commitId)
	if err != nil {
		return err
	}
	if db.commit != nil {
		db.commit.Free()
	}
	db.commit = commit
	return nil
}

// Pull downloads objects at the specified url and remote ref name,
// and updates the local ref of db.
// The uncommitted tree is left unchanged (ie uncommitted changes are
// not merged or rebased).
func (db *DB) Pull(url, ref string) error {
	if ref == "" {
		ref = db.ref
	}
	refspec := fmt.Sprintf("%s:%s", ref, db.ref)
	fmt.Printf("Creating anonymous remote url=%s refspec=%s\n", url, refspec)
	remote, err := db.repo.CreateAnonymousRemote(url, refspec)
	if err != nil {
		return err
	}
	defer remote.Free()
	if err := remote.Fetch(nil, fmt.Sprintf("libpack.pull %s %s", url, refspec)); err != nil {
		return err
	}
	return db.Update()
}

// Push uploads the committed contents of the db at the specified url and
// remote ref name. The remote ref is created if it doesn't exist.
func (db *DB) Push(url, ref string) error {
	if ref == "" {
		ref = db.ref
	}
	// The '+' prefix sets force=true,
	// so the remote ref is created if it doesn't exist.
	refspec := fmt.Sprintf("+%s:%s", db.ref, ref)
	remote, err := db.repo.CreateAnonymousRemote(url, refspec)
	if err != nil {
		return err
	}
	defer remote.Free()
	push, err := remote.NewPush()
	if err != nil {
		return fmt.Errorf("git_push_new: %v", err)
	}
	defer push.Free()
	if err := push.AddRefspec(refspec); err != nil {
		return fmt.Errorf("git_push_refspec_add: %v", err)
	}
	if err := push.Finish(); err != nil {
		return fmt.Errorf("git_push_finish: %v", err)
	}
	return nil
}

// Checkout populates the directory at dir with the committed
// contents of db. Uncommitted changes are ignored.
//
// As a convenience, if dir is an empty string, a temporary directory
// is created and returned, and the caller is responsible for removing it.
//
func (db *DB) Checkout(dir string) (checkoutDir string, err error) {
	if db.parent != nil {
		return db.parent.Checkout(path.Join(db.scope, dir))
	}
	head := db.Head()
	if head == nil {
		return "", fmt.Errorf("no head to checkout")
	}
	if dir == "" {
		dir, err = ioutil.TempDir("", "libpack-checkout-")
		if err != nil {
			return "", err
		}
		defer func() {
			if err != nil {
				os.RemoveAll(dir)
			}
		}()
	}
	stderr := new(bytes.Buffer)
	args := []string{
		"--git-dir", db.repo.Path(), "--work-tree", dir,
		"checkout", head.String(), ".",
	}
	cmd := exec.Command("git", args...)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s", stderr.String())
	}
	// FIXME: enforce scoping in the git checkout command instead
	// of here.
	d := path.Join(dir, db.scope)
	fmt.Printf("--> %s\n", d)
	return d, nil
}

// Checkout populates the directory at dir with the uncommitted
// contents of db.
// FIXME: this does not work properly at the moment.
func (db *DB) CheckoutUncommitted(dir string) error {
	if db.tree == nil {
		return fmt.Errorf("no tree")
	}
	tree, err := lookupSubtree(db.repo, db.tree, db.scope)
	if err != nil {
		return err
	}
	// If the tree is empty, checkout will fail and there is
	// nothing to do anyway
	if tree.EntryCount() == 0 {
		return nil
	}
	idx, err := ioutil.TempFile("", "libpack-index")
	if err != nil {
		return err
	}
	defer os.RemoveAll(idx.Name())
	readTree := exec.Command(
		"git",
		"--git-dir", db.repo.Path(),
		"--work-tree", dir,
		"read-tree", tree.Id().String(),
	)
	readTree.Env = append(readTree.Env, "GIT_INDEX_FILE="+idx.Name())
	stderr := new(bytes.Buffer)
	readTree.Stderr = stderr
	if err := readTree.Run(); err != nil {
		return fmt.Errorf("%s", stderr.String())
	}
	checkoutIndex := exec.Command(
		"git",
		"--git-dir", db.repo.Path(),
		"--work-tree", dir,
		"checkout-index",
	)
	checkoutIndex.Env = append(checkoutIndex.Env, "GIT_INDEX_FILE="+idx.Name())
	stderr = new(bytes.Buffer)
	checkoutIndex.Stderr = stderr
	if err := checkoutIndex.Run(); err != nil {
		return fmt.Errorf("%s", stderr.String())
	}
	return nil
}

// ExecInCheckout checks out the committed contents of the database into a
// temporary directory, executes the specified command in a new subprocess
// with that directory as the working directory, then removes the directory.
//
// The standard input, output and error streams of the command are the same
// as the current process's.
func (db *DB) ExecInCheckout(path string, args ...string) error {
	checkout, err := db.Checkout("")
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

// lookupBlob looks up an object at hash `id` in `repo`, and returns
// it as a git blob. If the object is not a blob, an error is returned.
func (db *DB) lookupBlob(id *git.Oid) (*git.Blob, error) {
	obj, err := db.repo.Lookup(id)
	if err != nil {
		return nil, err
	}
	if blob, ok := obj.(*git.Blob); ok {
		return blob, nil
	}
	return nil, fmt.Errorf("hash %v exist but is not a blob", id)
}

// lookupTree looks up an object at hash `id` in `repo`, and returns
// it as a git tree. If the object is not a tree, an error is returned.
func (db *DB) lookupTree(id *git.Oid) (*git.Tree, error) {
	return lookupTree(db.repo, id)
}

func lookupTree(r *git.Repository, id *git.Oid) (*git.Tree, error) {
	obj, err := r.Lookup(id)
	if err != nil {
		return nil, err
	}
	if tree, ok := obj.(*git.Tree); ok {
		return tree, nil
	}
	return nil, fmt.Errorf("hash %v exist but is not a tree", id)
}

func lookupBlob(r *git.Repository, id *git.Oid) (*git.Blob, error) {
	obj, err := r.Lookup(id)
	if err != nil {
		return nil, err
	}
	if blob, ok := obj.(*git.Blob); ok {
		return blob, nil
	}
	return nil, fmt.Errorf("hash %v exist but is not a blob", id)
}

// lookupCommit looks up an object at hash `id` in `repo`, and returns
// it as a git commit. If the object is not a commit, an error is returned.
func (db *DB) lookupCommit(id *git.Oid) (*git.Commit, error) {
	obj, err := db.repo.Lookup(id)
	if err != nil {
		return nil, err
	}
	if commit, ok := obj.(*git.Commit); ok {
		return commit, nil
	}
	return nil, fmt.Errorf("hash %v exist but is not a commit", id)
}

func lookupSubtree(repo *git.Repository, tree *git.Tree, name string) (*git.Tree, error) {
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

// emptyTree creates an empty Git tree and returns its ID
// (the ID will always be the same)
func emptyTree(repo *git.Repository) (*git.Oid, error) {
	builder, err := repo.TreeBuilder()
	if err != nil {
		return nil, err
	}
	return builder.Write()
}

package db

import (
	"bytes"
	"fmt"
	"io"
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
}

// Init initializes a new git-backed database from the following
// elements:
// * A bare git repository at `repo`
// * A git reference name `ref` (for example "refs/heads/foo")
// * An optional scope to expose only a subset of the git tree (for example "/myapp/v1")
func Init(repo, ref, scope string) (*DB, error) {
	r, err := git.InitRepository(repo, true)
	if err != nil {
		return nil, err
	}
	db := &DB{
		repo:  r,
		ref:   ref,
		scope: scope,
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

func (db *DB) Repo() *git.Repository {
	return db.repo
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
	err = subtree.Walk(func(name string, e *git.TreeEntry) int {
		obj, err := db.repo.Lookup(e.Id)
		if err != nil {
			handlerErr = err
			return -1
		}
		if err := h(path.Join(key, name), obj); err != nil {
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
// Uncommitted changes are left untouched (ie they are not merged
// or rebased).
func (db *DB) Update() error {
	tip, err := db.repo.LookupReference(db.ref)
	if err != nil {
		db.commit = nil
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
	if db.tree == nil {
		tree, err := db.commit.Tree()
		if err != nil {
			return err
		}
		db.tree = tree
	}
	return nil
}

// Get returns the value of the Git blob at path `key`.
// If there is no blob at the specified key, an error
// is returned.
func (db *DB) Get(key string) (string, error) {
	if db.tree == nil {
		return "", os.ErrNotExist
	}
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
	var (
		id  *git.Oid
		err error
	)
	// FIXME: libgit2 crashes if value is empty.
	// Work around this by shelling out to git.
	if value == "" {
		out, err := exec.Command("git", "--git-dir", db.repo.Path(), "hash-object", "-w", "--stdin").Output()
		if err != nil {
			return fmt.Errorf("git hash-object: %v", err)
		}
		id, err = git.NewOid(strings.Trim(string(out), " \t\r\n"))
		if err != nil {
			return fmt.Errorf("git newoid %v", err)
		}
	} else {
		id, err = db.repo.CreateBlobFromBuffer([]byte(value))
		if err != nil {
			return err
		}
	}
	// note: db.tree might be nil if this is the first entry
	newTree, err := treeUpdate(db.repo, db.tree, key, id)
	if err != nil {
		return fmt.Errorf("treeupdate: %v", err)
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

// List returns a list of object names at the subtree `key`.
// If there is no subtree at `key`, an error is returned.
func (db *DB) List(key string) ([]string, error) {
	if db.tree == nil {
		return []string{}, nil
	}
	e, err := db.tree.EntryByPath(path.Join(db.scope, key))
	if err != nil {
		return nil, err
	}
	subtree, err := db.lookupTree(e.Id)
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
	// FIXME: the ref might have been changed by another
	// process. We must implement either 1) reliable locking
	// or 2) a solid merge resolution strategy.
	// For now we simply assume the ref has not changed.
	var parents []*git.Commit
	if db.commit != nil {
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

// treeUpdate creates a new Git tree by adding a new object
// to it at the specified path.
// Intermediary subtrees are created as needed.
// If an object already exists at key or any intermediary path,
// it is overwritten.
//
// Since git trees are immutable, base is not modified. The new
// tree is returned.
// If an error is encountered, intermediary objects may be left
// behind in the git repository. It is the caller's responsibility
// to perform garbage collection, if any.
// FIXME: manage garbage collection, or provide a list of created
// objects.
func treeUpdate(repo *git.Repository, tree *git.Tree, key string, valueId *git.Oid) (*git.Tree, error) {
	key = path.Clean(key)
	key = strings.TrimLeft(key, "/") // Remove trailing slashes
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
	if base == "" {
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
		var old *git.TreeEntry
		if tree != nil {
			old = tree.EntryByName(leaf)
		}
		// If that subtree already exists, merge the new one in.
		if old != nil {
			oldObj, err := repo.Lookup(old.Id)
			if err != nil {
				return nil, err
			}
			oldTree, ok := oldObj.(*git.Tree)
			if !ok {
				return nil, fmt.Errorf("key %s has existing value of unexpected type: %#v", key, oldObj)
			}
			subTree = oldTree
			for i := uint64(0); i < oTree.EntryCount(); i++ {
				var err error
				e := oTree.EntryByIndex(i)
				subTree, err = treeUpdate(repo, subTree, e.Name, e.Id)
				if err != nil {
					return nil, err
				}
			}
		} else {
			subTree = oTree
		}
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
	subtree, err := treeUpdate(repo, nil, leaf, valueId)
	if err != nil {
		return nil, err
	}
	return treeUpdate(repo, tree, base, subtree.Id())
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
	entry, err := tree.EntryByPath(name)
	if err != nil {
		return nil, err
	}
	return lookupTree(repo, entry.Id)
}

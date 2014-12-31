package libpack

import (
	"fmt"
	"io"
	"sync"
)

type DB struct {
	r   *Repository
	ref string
	l   sync.RWMutex
}

// Repo returns the repository backing db.
func (db *DB) Repo() *Repository {
	return db.r
}

func (db *DB) Name() string {
	return db.ref
}

// Conveniences to query the underlying tree without explicitly
// fetching it first every time.
//
// DB.Get() is equivalent to DB.Tree().Get(), etc.

func (db *DB) Get(key string) (string, error) {
	t, err := db.Query().Run()
	if err != nil {
		return "", err
	}
	return t.Get(key)
}

func (db *DB) List(key string) ([]string, error) {
	t, err := db.Query().Run()
	if err != nil {
		return nil, err
	}
	return t.List(key)
}

func (db *DB) Dump(dst io.Writer) error {
	_, err := db.Query().Dump(dst).Run()
	return err
}

// Conveniences for basic write operations with (Set, Mkdir, Delete).
// These conveniences have auto-commit behavior. They are built on
// throwaway transactions under the hood.
//
// For the full power of transactions, use DB.Transaction instead

func (db *DB) Set(key, val string) (*Tree, error) {
	return db.Query().Set(key, val).Commit(db).Run()
}

func (db *DB) Mkdir(key string) (*Tree, error) {
	return db.Query().Mkdir(key).Commit(db).Run()
}

func (db *DB) Delete(key string) (*Tree, error) {
	return db.Query().Delete(key).Commit(db).Run()
}

func (db *DB) Query() *Pipeline {
	return NewPipeline(db.r).Query(db)
}

func (db *DB) getTree() (*Tree, error) {
	head, err := gitCommitFromRef(db.r.gr, db.ref)
	if err != nil {
		return nil, err
	}
	return db.r.TreeById(head.Id().String())
}

func (db *DB) setTree(t *Tree, old **Tree) (*Tree, error) {
	head, err := gitCommitFromRef(db.r.gr, db.ref)
	if isGitNoRefErr(err) {
		head = nil
	} else if err != nil {
		return nil, err
	}
	if old != nil {
		// Return the previous tree as a convenience for conflict management.
		*old, err = db.r.TreeById(head.Id().String())
		if err != nil {
			return nil, err
		}
	}
	commit, err := commitToRef(db.r.gr, t.Tree, head, db.ref, "")
	if err != nil {
		return nil, err
	}
	gt, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	newTree := &Tree{
		Tree: gt,
		r:    db.r,
	}
	if newTree.Hash() != t.Hash() {
		return newTree, fmt.Errorf("Mismatched hash: %s instead of %s", newTree.Hash(), t.Hash())
	}
	return newTree, nil
}

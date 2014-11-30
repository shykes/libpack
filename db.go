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
	return db.Transaction().Set(key, val).Run()
}

func (db *DB) Mkdir(key string) (*Tree, error) {
	return db.Transaction().Mkdir(key).Run()
}

func (db *DB) Delete(key string) (*Tree, error) {
	return db.Transaction().Delete(key).Run()
}

// A transaction is just a Pipeline with some glue to commit
// after a successful run. In other words: tree.go and pipeline.go
// do all the glue work for us. And git does all the really hard work.
// It feels good to be the top of the stack.

func (db *DB) Transaction() *Pipeline {
	return db.pipeline(true)
}

// A Query is just a pipeline which *does not* commit at the end.

func (db *DB) Query() *Pipeline {
	return db.pipeline(false)
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
	if err != nil {
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

func (db *DB) pipeline(commit bool) *Pipeline {
	// FIXME: this can be done more cleanly by making Pipeline a linked list
	// of abstract PipelineStep interfaces.
	// The builtin steps (Add, Set, Delete etc.) can be added with the same
	// convenience methods. But another call would allow adding arbitrary
	// steps (which would be anything implementing the interface)
	return NewPipeline(db.r).OnRun(func(p *Pipeline) (*Tree, error) {
		// FIXME: we can add a scope pre-processor here.
		// Get the current from the db
		tree, err := db.getTree()
		if err != nil {
			return nil, err
		}
		// Use that value as the input of the pipeline
		out, err := concat(tree.Pipeline(), p).Run()
		// If commit==false, just return the result
		if !commit {
			return out, err
		}
		// If commit==true, write the result back to the db
		return db.setTree(out, nil)
	})
}

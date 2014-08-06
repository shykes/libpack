package db

import (
	"fmt"
	"os"
	"path"
	"time"

	git "github.com/libgit2/git2go"
)

type DB struct {
	repo    *git.Repository
	commit  *git.Commit
	ref     string
	scope   string
	changes Tree
}

type change struct {
	Op   changeOp
	Name string
	Id   *git.Oid
}

type changeOp int

const (
	opAdd changeOp = iota
	opDel
)

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

func (db *DB) Free() {
	db.repo.Free()
	if db.commit != nil {
		db.commit.Free()
	}
}

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
	return nil
}

// High-level interface: Get, Set, Cd, Mkdir
// Accepts paths.

func (db *DB) Get(key string) (string, error) {
	if db.commit == nil {
		return "", os.ErrNotExist
	}
	tree, err := db.commit.Tree()
	if err != nil {
		return "", err
	}
	defer tree.Free()
	e, err := tree.EntryByPath(path.Join(db.scope, key))
	if err != nil {
		return "", err
	}
	blob, err := db.lookupBlob(e.Id)
	if err != nil {
		return "", err
	}
	return string(blob.Contents()), nil
}

func (db *DB) IsDir(key string) (bool, error) {
	if db.commit == nil {
		return false, nil
	}
	tree, err := db.commit.Tree()
	if err != nil {
		return false, err
	}
	e, err := tree.EntryByPath(path.Join(db.scope, key))
	if err != nil {
		return "", err
	}
	tree, err := db.lookupTree(e.Id)
	if err != nil {
		return false, nil
	}
	defer tree.Free()
	return true, nil
}

func (db *DB) Set(key, value string) error {
	id, err := db.repo.CreateBlobFromBuffer([]byte(value))
	if err != nil {
		return err
	}
	if err := db.changes.Update(key,
	db.changes = append(db.changes, &change{Op: opAdd, Name: key, Id: id})
	return nil
}

func (db *DB) List(key string) ([]string, error) {
	if db.commit == nil {
		return []string{}, nil
	}
	tree, err := db.commit.Tree()
	if err != nil {
		return nil, err
	}
	defer tree.Free()
	e, err := tree.EntryByPath(path.Join(db.scope, key))
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

func (db *DB) Commit(msg string) error {
	var (
		tb  *git.TreeBuilder
		err error
	)
	if db.commit != nil {
		tree, err := db.commit.Tree()
		if err != nil {
			return err
		}
		defer tree.Free()
		tb, err = db.repo.TreeBuilderFromTree(tree)
		if err != nil {
			return err
		}
	} else {
		tb, err = db.repo.TreeBuilder()
		if err != nil {
			return err
		}
	}
	defer tb.Free()
	for _, ch := range db.changes {
		if ch.Op == opAdd {
			parts := pathParts(ch.Name)
			for i, dir := range parts[:len(parts - 1)] {
				// insert the intermediary directories if they don't exist
				if isDir, err := db.IsDir(path.Join(parts[:i+1]...)); err != nil {
					return err
				} else if !isDir {
					if err := tb.Insert(, ch.Id, 040000); err != nil {
						return err
			}
					
				}
				if err 
			}
			if err := tb.Insert(ch.Name, ch.Id, 0100644); err != nil {
				return err
			}
		} else if ch.Op == opDel {
			if err := tb.Remove(ch.Name); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("invalid op: %s", ch.Op)
		}
	}
	newTreeId, err := tb.Write()
	if err != nil {
		return err
	}
	newTree, err := db.lookupTree(newTreeId)
	if err != nil {
		return err
	}
	var parents []*git.Commit
	if db.commit != nil {
		parents = append(parents, db.commit)
	}
	commitId, err := db.repo.CreateCommit(
		db.ref,
		&git.Signature{"libpack", "libpack", time.Now()}, // author
		&git.Signature{"libpack", "libpack", time.Now()}, // committer
		msg,
		newTree,    // git tree to commit
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
	obj, err := db.repo.Lookup(id)
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

func pathParts(p string) (parts []string) {
	p = path.Clean(p)
	// path.Clean("") returns "."
	if p == "." || p == "/" {
		return []string{}
	}
	p = strings.TrimLeft(p, "/")
	return strings.Split(p, "/")
}


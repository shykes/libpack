package libpack

import (
	"fmt"
	"sync"

	git "github.com/libgit2/git2go"
)

// DB is a simple git-backed database.
type DB struct {
	r   *Repository
	ref string
	l   sync.RWMutex
}

func (db *DB) Get() (*Tree, error) {
	head, err := gitCommitFromRef(db.r.gr, db.ref)
	if err != nil {
		return nil, err
	}
	return db.r.TreeById(head.Id().String())
}

func (db *DB) Watch() (*Tree, chan *Tree, error) {
	// FIXME
	return nil, nil, fmt.Errorf("not implemented")
}

func (db *DB) Commit(t *Tree, msg string) (*Tree, error) {
	if t == nil {
		return t, nil
	}
	head, err := gitCommitFromRef(db.r.gr, db.ref)
	if err != nil {
		return nil, err
	}
	commit, err := commitToRef(db.r.gr, t.Tree, head, db.ref, msg)
	if err != nil {
		return nil, err
	}
	gt, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	return &Tree{
		Tree: gt,
		r:    t.r,
	}, nil
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
	remote, err := db.r.gr.CreateAnonymousRemote(url, refspec)
	if err != nil {
		return err
	}
	defer remote.Free()
	if err := remote.Fetch(nil, nil, fmt.Sprintf("libpack.pull %s %s", url, refspec)); err != nil {
		return err
	}
	return nil
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
	remote, err := db.r.gr.CreateAnonymousRemote(url, refspec)
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

// lookupTree looks up an object at hash `id` in `repo`, and returns
// it as a git tree. If the object is not a tree, an error is returned.
func (db *DB) lookupTree(id *git.Oid) (*git.Tree, error) {
	return lookupTree(db.r.gr, id)
}

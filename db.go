package libpack

import (
	"fmt"
	"sync"

	git "github.com/libgit2/git2go"
)

// DB is a simple git-backed database.
type DB struct {
	r   *git.Repository
	ref string
	l   sync.RWMutex
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

// Open opens an existing repository. See Init() for parameters.
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

func OpenOrInit(repo, ref string) (*DB, error) {
	if db, err := Open(repo, ref); err == nil {
		return db, err
	}
	return Init(repo, ref)
}

func newRepo(repo *git.Repository, ref string) (*DB, error) {
	db := &DB{
		r:   repo,
		ref: ref,
	}
	return db, nil
}

// Free must be called to release resources when a database is no longer
// in use.
// This is required in addition to Golang garbage collection, because
// of the libgit2 C bindings.
func (db *DB) Free() {
	db.r.Free()
}

func (db *DB) Repo() *git.Repository {
	return db.r
}

func (db *DB) Get() (*Tree, error) {
	head, err := db.head()
	if err != nil {
		return nil, err
	}
	return TreeFromGit(db.r, head.Id())
}

func (db *DB) head() (*git.Commit, error) {
	tip, err := db.r.LookupReference(db.ref)
	if err != nil {
		return nil, err
	}
	return lookupCommit(db.r, tip.Target())
}

func (db *DB) Watch() (*Tree, chan *Tree, error) {
	// FIXME
	return nil, nil, fmt.Errorf("not implemented")
}

func (db *DB) Commit(t *Tree, msg string) (*Tree, error) {
	if t == nil {
		return t, nil
	}
	head, err := db.head()
	if err != nil {
		return nil, err
	}
	commit, err := CommitToRef(db.r, t.Tree, head, db.ref, msg)
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
	remote, err := db.r.CreateAnonymousRemote(url, refspec)
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
	remote, err := db.r.CreateAnonymousRemote(url, refspec)
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
	return lookupTree(db.r, id)
}

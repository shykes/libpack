package libpack

import (
	"fmt"
	"sync"
	"time"

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

// FIXME: move to git.go
func CommitToRef(r *git.Repository, tree *git.Tree, parent *git.Commit, refname, msg string) (*git.Commit, error) {
	// Retry loop in case of conflict
	// FIXME: use a custom inter-process lock as a first attempt for performance
	var (
		needMerge bool
		tmpCommit *git.Commit
	)
	for {
		if !needMerge {
			// Create simple commit
			commit, err := mkCommit(r, refname, msg, tree, parent)
			if isGitConcurrencyErr(err) {
				needMerge = true
				continue
			}
			return commit, err
		} else {
			if tmpCommit == nil {
				var err error
				// Create a temporary intermediary commit, to pass to MergeCommits
				// NOTE: this commit will not be part of the final history.
				tmpCommit, err = mkCommit(r, "", msg, tree, parent)
				if err != nil {
					return nil, err
				}
				defer tmpCommit.Free()
			}
			// Lookup tip from ref
			tip := lookupTip(r, refname)
			if tip == nil {
				// Ref may have been deleted after previous merge error
				needMerge = false
				continue
			}

			// Merge simple commit with the tip
			opts, err := git.DefaultMergeOptions()
			if err != nil {
				return nil, err
			}
			idx, err := r.MergeCommits(tmpCommit, tip, &opts)
			if err != nil {
				return nil, err
			}
			conflicts, err := idx.ConflictIterator()
			if err != nil {
				return nil, err
			}
			defer conflicts.Free()
			for {
				c, err := conflicts.Next()
				if isGitIterOver(err) {
					break
				} else if err != nil {
					return nil, err
				}
				if c.Our != nil {
					idx.RemoveConflict(c.Our.Path)
					if err := idx.Add(c.Our); err != nil {
						return nil, fmt.Errorf("error resolving merge conflict for '%s': %v", c.Our.Path, err)
					}
				}
			}
			mergedId, err := idx.WriteTreeTo(r)
			if err != nil {
				return nil, fmt.Errorf("WriteTree: %v", err)
			}
			mergedTree, err := lookupTree(r, mergedId)
			if err != nil {
				return nil, err
			}
			// Create new commit from merged tree (discarding simple commit)
			commit, err := mkCommit(r, refname, msg, mergedTree, parent, tip)
			if isGitConcurrencyErr(err) {
				// FIXME: enforce a maximum number of retries to avoid infinite loops
				continue
			}
			return commit, err
		}
	}
	return nil, fmt.Errorf("too many failed merge attempts, giving up")
}

func mkCommit(r *git.Repository, refname string, msg string, tree *git.Tree, parent *git.Commit, extraParents ...*git.Commit) (*git.Commit, error) {
	var parents []*git.Commit
	if parent != nil {
		parents = append(parents, parent)
	}
	if len(extraParents) > 0 {
		parents = append(parents, extraParents...)
	}
	id, err := r.CreateCommit(
		refname,
		&git.Signature{"libpack", "libpack", time.Now()}, // author
		&git.Signature{"libpack", "libpack", time.Now()}, // committer
		msg,
		tree, // git tree to commit
		parents...,
	)
	if err != nil {
		return nil, err
	}
	return lookupCommit(r, id)
}

func isGitConcurrencyErr(err error) bool {
	gitErr, ok := err.(*git.GitError)
	if !ok {
		return false
	}
	return gitErr.Class == 11 && gitErr.Code == -15
}

func isGitIterOver(err error) bool {
	gitErr, ok := err.(*git.GitError)
	if !ok {
		return false
	}
	return gitErr.Code == git.ErrIterOver
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

// lookupBlob looks up an object at hash `id` in `repo`, and returns
// it as a git blob. If the object is not a blob, an error is returned.
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

// lookupTip looks up the object referenced by refname, and returns it
// as a Commit object. If the reference does not exist, or if object is
// not a commit, nil is returned. Other errors cannot be detected.
func lookupTip(r *git.Repository, refname string) *git.Commit {
	ref, err := r.LookupReference(refname)
	if err != nil {
		return nil
	}
	commit, err := lookupCommit(r, ref.Target())
	if err != nil {
		return nil
	}
	return commit
}

// lookupCommit looks up an object at hash `id` in `repo`, and returns
// it as a git commit. If the object is not a commit, an error is returned.
func lookupCommit(r *git.Repository, id *git.Oid) (*git.Commit, error) {
	obj, err := r.Lookup(id)
	if err != nil {
		return nil, err
	}
	if commit, ok := obj.(*git.Commit); ok {
		return commit, nil
	}
	return nil, fmt.Errorf("hash %v exist but is not a commit", id)
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

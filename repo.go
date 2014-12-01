package libpack

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	git "github.com/libgit2/git2go"
)

type Repository struct {
	gr *git.Repository // `gr` stands for "git repository"
}

func Init(dir string, create bool) (*Repository, error) {
	var (
		err error
		gr  *git.Repository
	)
	gr, err = git.OpenRepository(dir)
	if err == nil {
		return newRepository(gr)
	}
	if !create {
		return nil, err
	}
	gr, err = git.InitRepository(dir, true)
	if err == nil {
		return newRepository(gr)
	}
	return nil, err
}

func newRepository(gr *git.Repository) (*Repository, error) {
	return &Repository{
		gr: gr,
	}, nil
}

func (r *Repository) DB(ref string) (*DB, error) {
	// As a convenience, if no ref name is given, we generate a
	// unique one.
	if ref == "" {
		ref = fmt.Sprintf("refs/heads/%s", randomString())

		t, err := r.EmptyTree()
		if err != nil {
			return nil, err
		}

		if _, err := commitToRef(r.gr, t.Tree, nil, ref, "new head"); err != nil {
			return nil, err
		}
	}

	return &DB{
		r:   r,
		ref: ref,
	}, nil
}

func randomString() string {
	id := make([]byte, 32)

	if _, err := io.ReadFull(rand.Reader, id); err != nil {
		panic(err) // This shouldn't happen
	}
	return hex.EncodeToString(id)
}

func (r *Repository) EmptyTree() (*Tree, error) {
	id, err := emptyTree(r.gr)
	if err != nil {
		return nil, err
	}
	gt, err := lookupTree(r.gr, id)
	if err != nil {
		return nil, err
	}
	return &Tree{
		Tree: gt,
		r:    r,
	}, nil
}

func (r *Repository) TreeById(tid string) (*Tree, error) {
	id, err := git.NewOid(tid)
	if err != nil {
		return nil, err
	}
	gt, err := lookupTree(r.gr, id)
	if err == nil {
		return &Tree{
			Tree: gt,
			r:    r,
		}, nil
	}
	gc, err := lookupCommit(r.gr, id)
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

// Pull downloads objects at the specified url and remote ref name,
// and updates the local ref of db.
// The uncommitted tree is left unchanged (ie uncommitted changes are
// not merged or rebased).
func (r *Repository) Pull(url, fromref, toref string) error {
	if fromref == "" {
		fromref = toref
	}
	refspec := fmt.Sprintf("%s:%s", fromref, toref)
	fmt.Printf("Creating anonymous remote url=%s refspec=%s\n", url, refspec)
	remote, err := r.gr.CreateAnonymousRemote(url, refspec)
	if err != nil {
		return err
	}
	defer remote.Free()
	if err := remote.Fetch(nil, nil, fmt.Sprintf("libpack.pull %s %s", url, refspec)); err != nil {
		return err
	}
	return nil
}

// Push uploads the committed contents of the repository at the specified url and
// remote ref name. The remote ref is created if it doesn't exist.
func (r *Repository) Push(url, fromref, toref string) error {
	if toref == "" {
		toref = fromref
	}
	// The '+' prefix sets force=true,
	// so the remote ref is created if it doesn't exist.
	refspec := fmt.Sprintf("+%s:%s", fromref, toref)
	remote, err := r.gr.CreateAnonymousRemote(url, refspec)
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

// Free must be called to release resources when a repository is no longer
// in use.
// This is required in addition to Golang garbage collection, because
// of the libgit2 C bindings.
func (r *Repository) Free() {
	r.gr.Free()
}

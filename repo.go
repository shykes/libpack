package libpack

import (
	"fmt"

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

func (r *Repository) DB(ref string) *DB {
	return &DB{
		r:   r,
		ref: ref,
	}
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

// Free must be called to release resources when a repository is no longer
// in use.
// This is required in addition to Golang garbage collection, because
// of the libgit2 C bindings.
func (r *Repository) Free() {
	r.gr.Free()
}

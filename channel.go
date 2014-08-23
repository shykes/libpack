package libpack

import (
	"fmt"

	git "github.com/libgit2/git2go"
)

// A channel is a versioned collection of packages.
// Each package is a json file stored as a blob in a git tree.
// The git tree is versioned in a ref.
// Packages are self-describing. Each package must be stored
// at the path corresponding to its name. Misplaced packages
// must be ignored.

type Channel struct {
	*DB
}

func NewChannel(db *DB) *Channel {
	return &Channel{db}
}

func (c *Channel) Get(name string) (*Package, error) {
	val, err := c.DB.Get(name)
	if err != nil {
		return nil, err
	}
	return DecodePkg([]byte(val), name)
}

func (c *Channel) Iterate(h func(*Package)) error {
	return c.DB.Walk("/", func(name string, obj git.Object) error {
		blob, isBlob := obj.(*git.Blob)
		if !isBlob {
			return nil
		}
		pkg, err := DecodePkg(blob.Contents(), name)
		if err != nil {
			// Ignore incorrect packages
			return nil
		}
		h(pkg)
		return nil
	})
}

func (c *Channel) Add(pkg *Package) error {
	if err := c.DB.Set(pkg.Path(), pkg.String()); err != nil {
		return err
	}
	return c.DB.Commit(fmt.Sprintf("add %s", pkg.Path()))
}

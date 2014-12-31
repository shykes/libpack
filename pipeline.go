package libpack

import (
	"container/list"
	"fmt"
	"io"
)

type Query interface {
	// FIXME: support a stream of multiple values
	// FIXME: support Value results instead of just Tree
	Run() (*Tree, error)
}

// A Pipeline defines a sequence of operations which can be run
// to produce a libpack object.
// Pipelines make it easy to assemble trees of arbitrary complexity
// with relatively few lines of code.
// For example:
//
//   p := NewPipeline().Set("foo", "bar").Mkdir("/directory")
//   tree, _ := p.Run()
//
// Pipelines can be nested with AddQuery:
//
//   p1 := NewPipeline().Set("hello", "world")
//   p2 := NewPipeline().AddQuery("subdir", p1, true)
//   combotree, _ := p2.Run()
//
type Pipeline struct {
	*list.List
	r *Repository
}

// Op is an operation in a pipeline.
// Each operation receives an input tree, and produces an output tree
// or an error.
// The output of an operation is passed as input to the next operation in the
// pipeline, and so on.
type Op func(in *Tree) (out *Tree, err error)

func NewPipeline(r *Repository) *Pipeline {
	return &Pipeline{
		List: list.New(),
		r:    r,
	}
}

func (p *Pipeline) PushBackPipeline(other *Pipeline) {
	p.PushBackList(other.List)
}

func (p *Pipeline) PushFrontPipeline(other *Pipeline) {
	p.PushFrontList(other.List)
}

// Run runs each step of the pipeline in sequence, each time passing
// the output of step N as input to step N+1.
// If an error is encountered, the pipeline is aborted.
func (p *Pipeline) Run() (val *Tree, err error) {
	val, err = p.r.EmptyTree()
	if err != nil {
		return
	}
	for e := p.Front(); e != nil; e = e.Next() {
		op, ok := e.Value.(Op)
		if !ok {
			// Skip values which are not Ops.
			// This is easier than overriding PushBack, PushFront etc to validate.
			continue
		}
		val, err = op(val)
		if err != nil {
			return
		}
	}
	return
}

// As a convenience, Get runs the pipeline, then calls Get on the
// resulting tree.
func (p *Pipeline) Get(key string) (string, error) {
	out, err := p.Run()
	if err != nil {
		return "", err
	}
	return out.Get(key)
}

// Empty appends a pipeline operation which ignores is input, and
// always returns an empty tree.
func (p *Pipeline) Empty() *Pipeline {
	p.PushBack(OpEmpty())
	return p
}

func OpEmpty() Op {
	return func(in *Tree) (*Tree, error) {
		return in.Repo().EmptyTree()
	}
}

// Nop appends a pipeline operation which simply passes its input tree,
// unmodified, as output.
func (p *Pipeline) Nop() *Pipeline {
	p.PushBack(OpNop())
	return p
}

func OpNop() Op {
	return func(in *Tree) (*Tree, error) {
		return in, nil
	}
}

// Add appends a pipeline operation which adds the specified overlay tree
// to the input tree at the specified key.
// If merge is true, the contents of the overlay tree are merged into the
// existing contents at that key, at the file granularity.
// If merge is false, any pre-existing content at that key is removed.
func (p *Pipeline) Add(key string, overlay *Tree, merge bool) *Pipeline {
	p.PushBack(OpAdd(key, overlay, merge))
	return p
}

func OpAdd(key string, overlay *Tree, merge bool) Op {
	return func(in *Tree) (*Tree, error) {
		return in.Add(key, overlay, merge)
	}
}

// AddQuery appends a pipeline operation which runs the specified query,
// and adds the resulting tree through an `add` operation.
func (p *Pipeline) AddQuery(key string, q Query, merge bool) *Pipeline {
	p.PushBack(OpAddQuery(key, q, merge))
	return p
}

func OpAddQuery(key string, q Query, merge bool) Op {
	return func(in *Tree) (*Tree, error) {
		overlay, err := q.Run()
		if err != nil {
			return nil, err
		}
		return OpAdd(key, overlay, merge)(in)
	}
}

// Delete appends a pipeline operation which deletes the specified key
// from the input tree.
func (p *Pipeline) Delete(key string) *Pipeline {
	p.PushBack(OpDelete(key))
	return p
}

func OpDelete(key string) Op {
	return func(in *Tree) (*Tree, error) {
		return in.Delete(key)
	}
}

// Mkdir appends a pipeline operation which creates an empty tree
// at the specified key in the input tree.
// If a tree is already present at the specified key, the input tree
// is passed through unmidified.
func (p *Pipeline) Mkdir(key string) *Pipeline {
	p.PushBack(OpMkdir(key))
	return p
}

func OpMkdir(key string) Op {
	return func(in *Tree) (*Tree, error) {
		return in.Mkdir(key)
	}
}

// Mkdir appends a pipeline operation which sets the specified key
// and value in the input tree, and outputs the resulting tree.
// If a value already exists at that key, it is overwritten.
func (p *Pipeline) Set(key, value string) *Pipeline {
	p.PushBack(OpSet(key, value))
	return p
}

func OpSet(key, val string) Op {
	return func(in *Tree) (*Tree, error) {
		return in.Set(key, val)
	}
}

// Scope appends a pipeline operation which fetches a sub-tree
// from its input tree at the specified key, and returns that
// sub-tree as its output.
func (p *Pipeline) Scope(key string) *Pipeline {
	p.PushBack(OpScope(key))
	return p
}

func OpScope(key string) Op {
	return func(in *Tree) (*Tree, error) {
		return in.Scope(key)
	}
}

// Walk appends a pipeline operation which calls Tree.Walk on its input tree
// with the specified callback as argument, then passes the unmodified
// tree as output.
func (p *Pipeline) Walk(h func(key string, entry Value) error) *Pipeline {
	p.PushBack(OpWalk(h))
	return p
}

func OpWalk(h func(key string, entry Value) error) Op {
	return func(in *Tree) (*Tree, error) {
		return in, in.Walk(h)
	}
}

// Dump appends a pipeline operation which calls Tree.Dump on its input tree
// with the specified destination stream as argument, then passes the unmodified
// tree as output.
func (p *Pipeline) Dump(dst io.Writer) *Pipeline {
	p.PushBack(OpDump(dst))
	return p
}

func OpDump(dst io.Writer) Op {
	return func(in *Tree) (*Tree, error) {
		return in, in.Dump(dst)
	}
}

func (p *Pipeline) AssertEq(key, val string) *Pipeline {
	p.PushBack(OpAssertEq(key, val))
	return p
}

func OpAssertEq(key, val string) Op {
	return func(in *Tree) (*Tree, error) {
		v, err := in.Get(key)
		if err != nil {
			return nil, err
		}
		if v != val {
			return nil, fmt.Errorf("assertion failed: '%v == %v'", v, val)
		}
		return in, nil
	}
}

func (p *Pipeline) AssertNotExist(key string) *Pipeline {
	p.PushBack(OpAssertNotExist(key))
	return p
}

func OpAssertNotExist(key string) Op {
	return func(in *Tree) (*Tree, error) {
		_, err := in.Get(key)
		if err == nil {
			return nil, fmt.Errorf("assertion failed: '%s is not set'", key)
		}
		return in, nil
	}
}

// Query appends a pipeline operation which ignores its input, queries the
// specified database for its content, and returns the result as its output.
func (p *Pipeline) Query(db *DB) *Pipeline {
	// FIXME: rename to FromDB for clarity (there is confusion with the Query interface)
	p.PushBack(OpQuery(db))
	return p
}

func OpQuery(db *DB) Op {
	return func(in *Tree) (*Tree, error) {
		t, err := db.getTree()
		// If the DB doesn't exist, return an empty tree.
		if isGitNoRefErr(err) {
			empty, err := db.Repo().EmptyTree()
			if err != nil {
				return nil, err
			}
			return empty, nil
		} else if err != nil {
			return nil, err
		}
		return t, nil
	}
}

// Commit appends a pipeline operation which
func (p *Pipeline) Commit(db *DB) *Pipeline {
	// FIXME: rename to ToDB for consistency with Query/FromDB
	p.PushBack(OpCommit(db))
	return p
}

func OpCommit(db *DB) Op {
	return func(in *Tree) (*Tree, error) {
		return db.setTree(in, nil)
	}
}

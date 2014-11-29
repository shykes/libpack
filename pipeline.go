package libpack

import (
	"fmt"
)

// A Pipeline defines a sequence of operations which can be run
// to produce a Git Tree.
// Pipelines make it easy to assemble trees of arbitrary complexity
// with relatively few lines of code.
// For example:
//
//   p := NewPipeline().Set("foo", "bar").Mkdir("/directory")
//   tree, _ := p.Run()
//
// Pipelines can be nested with Add:
//
//   p1 := NewPipeline().Set("hello", "world")
//   p2 := NewPipeline().Add("subdir", p1, true)
//   combotree, _ := p2.Run()
//
type Pipeline struct {
	prev *Pipeline
	op   TreeOp
	arg  interface{}
}

type addArg struct {
	key     string
	overlay *Tree
	merge   bool
}

type walkArg WalkHandler

// A TreeOp defines an individual operation operation in a pipeline.
type TreeOp int

const (
	OpEmpty TreeOp = iota
	OpNop
	OpSet
	OpMkdir
	OpAdd
	OpScope
	OpDelete
	OpWalk
)

// Set appends a new `set` instruction to a pipeline, and
// returns the new combined pipeline.
// `set` writes `value` in a blob at path `key` in input trees.
func (t *Pipeline) Set(key, value string) *Pipeline {
	return t.setPrev(OpSet, []string{key, value})
}

// Add appends a new `add` instruction to a pipeline, and
// returns the new combined pipeline.
// `add` inserts a git object in the input tree, at the pat 'key'.
// The following types are supported for `val`:
//  - git.Object: the specified object is added
//  - *git.Oid: the object at the specified ID is added
//  - *Pipeline: the specified pipeline is run, and the result is added
func (t *Pipeline) Add(key string, overlay *Tree, merge bool) *Pipeline {
	return t.setPrev(OpAdd, &addArg{
		key:     key,
		overlay: overlay,
		merge:   merge,
	})
}

func (t *Pipeline) Walk(h func(key string, entry Value) error) *Pipeline {
	return t.setPrev(OpWalk, h)
}

// Delete appends a new `delete` instruction to a pipeline, then returns the
// combined Pipeline.
func (t *Pipeline) Delete(key string) *Pipeline {
	return t.setPrev(OpDelete, key)
}

// Mkdir appends a new `mkdir` instruction to a pipeline, and
// returns the new combined pipeline.
// `mkdir` inserts an empty subtree in the input tree, at
// the path `key`.
func (t *Pipeline) Mkdir(key string) *Pipeline {
	return t.setPrev(OpMkdir, key)
}

func (t *Pipeline) Scope(key string) *Pipeline {
	return t.setPrev(OpScope, key)
}

// Run runs each step of the pipeline in sequence, each time passing
// the output of step N as input to step N+1.
// If an error is encountered, the pipeline is aborted.
func (t *Pipeline) Run() (*Tree, error) {
	var in *Tree
	// Call the previous operation before our own
	// (unless the current operation is Empty or Nop, since they would
	// discard the result anyway)
	if t.prev != nil && t.op != OpEmpty && t.op != OpNop {
		prevOut, err := t.prev.Run()
		if err != nil {
			return nil, err
		}
		in = prevOut
	}
	switch t.op {
	case OpEmpty:
		{
			return in.Empty()
		}
	case OpNop:
		{
			return in, nil
		}
	case OpAdd:
		{
			arg, ok := t.arg.(*addArg)
			if !ok {
				return nil, fmt.Errorf("add: invalid argument: %v", t.arg)
			}
			return in.Add(arg.key, arg.overlay, arg.merge)
		}
	case OpDelete:
		{
			key, ok := t.arg.(string)
			if !ok {
				return nil, fmt.Errorf("delete: invalid argument: %v", t.arg)
			}
			return in.Delete(key)
		}
	case OpMkdir:
		{
			key, ok := t.arg.(string)
			if !ok {
				return nil, fmt.Errorf("mkdir: invalid argument: %v", t.arg)
			}
			return in.Mkdir(key)
		}
	case OpSet:
		{
			kv, ok := t.arg.([]string)
			if !ok {
				return nil, fmt.Errorf("invalid argument")
			}
			if len(kv) != 2 {
				return nil, fmt.Errorf("invalid argument")
			}
			return in.Set(kv[0], kv[1])
		}
	case OpScope:
		{
			key, ok := t.arg.(string)
			if !ok {
				return nil, fmt.Errorf("mkdir: invalid argument: %v", t.arg)
			}
			return in.Scope(key)
		}
	case OpWalk:
		{
			h, ok := t.arg.(walkArg)
			if !ok {
				return nil, fmt.Errorf("mkdir: invalid argument: %v", t.arg)
			}
			return in, in.Walk(WalkHandler(h))
		}
	}
	return nil, fmt.Errorf("invalid op: %v", t.op)
}

func (t *Pipeline) setPrev(op TreeOp, arg interface{}) *Pipeline {
	return &Pipeline{
		prev: t,
		op:   op,
		arg:  arg,
	}
}

package libpack

import (
	"fmt"
	"io"
)

// A Pipeline defines a sequence of operations which can be run
// to produce a libpack object.
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
	r    *Repository // needed for Empty() when there is no input
	prev *Pipeline
	op   Op
	arg  interface{}
	run  PipelineHandler
}

type PipelineHandler func(*Pipeline) (*Tree, error)

func NewPipeline(r *Repository) *Pipeline {
	return &Pipeline{
		r:  r,
		op: OpNop,
	}
}

type addArg struct {
	key     string
	overlay interface{}
	merge   bool
}

type walkArg WalkHandler
type dumpArg io.Writer

// An Op defines an individual operation in a pipeline.
type Op int

const (
	OpEmpty Op = iota
	OpNop
	OpSet
	OpMkdir
	OpAdd
	OpScope
	OpDelete
	OpWalk
	OpDump
)

func (t *Pipeline) OnRun(run PipelineHandler) *Pipeline {
	return &Pipeline{
		prev: t.prev,
		op:   t.op,
		arg:  t.arg,
		run:  run,
	}
}

// Set appends a new `set` instruction to a pipeline, and
// returns the new combined pipeline.
// `set` writes `value` in a blob at path `key` in input trees.
func (t *Pipeline) Set(key, value string) *Pipeline {
	return t.setPrev(OpSet, []string{key, value})
}

func (t *Pipeline) Empty() *Pipeline {
	return t.setPrev(OpEmpty, nil)
}

// Add appends a new `add` instruction to a pipeline, and
// returns the new combined pipeline.
// `add` inserts a git object in the input tree, at the pat 'key'.
// The following types are supported for `val`:
//  - git.Object: the specified object is added
//  - *git.Oid: the object at the specified ID is added
//  - *Pipeline: the specified pipeline is run, and the result is added
func (t *Pipeline) Add(key string, overlay interface{}, merge bool) *Pipeline {
	return t.setPrev(OpAdd, &addArg{
		key:     key,
		overlay: overlay,
		merge:   merge,
	})
}

func (t *Pipeline) Walk(h func(key string, entry Value) error) *Pipeline {
	return t.setPrev(OpWalk, h)
}

func (t *Pipeline) Dump(dst io.Writer) *Pipeline {
	return t.setPrev(OpDump, dst)
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
func (t *Pipeline) Run() (out *Tree, err error) {
	if t.run != nil {
		return t.run(t.OnRun(nil))
	}
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
			return t.r.EmptyTree()
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
			switch overlay := arg.overlay.(type) {
			case *Tree:
				{
					return in.Add(arg.key, overlay, arg.merge)
				}
			case *Pipeline:
				{
					out, err := overlay.Run()
					if err != nil {
						return nil, err
					}
					return in.Add(arg.key, out, arg.merge)
				}
			}
			return nil, fmt.Errorf("invalid overlay argument to add: %#v", arg.overlay)
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
				return nil, fmt.Errorf("invalid argument: %v", t.arg)
			}
			return in.Scope(key)
		}
	case OpWalk:
		{
			h, ok := t.arg.(walkArg)
			if !ok {
				return nil, fmt.Errorf("invalid argument: %v", t.arg)
			}
			return in, in.Walk(WalkHandler(h))
		}
	case OpDump:
		{
			dst, ok := t.arg.(dumpArg)
			if !ok {
				return nil, fmt.Errorf("invalid argument: %v", t.arg)
			}
			return in, in.Dump(dst)
		}
	}
	return nil, fmt.Errorf("invalid op: %v", t.op)
}

func (t *Pipeline) setPrev(op Op, arg interface{}) *Pipeline {
	return &Pipeline{
		prev: t,
		op:   op,
		arg:  arg,
	}
}

func concat(p1, p2 *Pipeline) *Pipeline {
	if p1 == nil {
		return p2
	}
	if p2 == nil {
		return p1
	}
	// FIXME: use a linked list to make this cheaper
	var step *Pipeline
	for step = p2; step.prev != nil; step = step.prev {
	}
	step.prev = p1
	return p2
}

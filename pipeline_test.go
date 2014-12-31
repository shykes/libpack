package libpack

import (
	"bytes"
	"testing"
)

const EmptyTreeId string = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

func TestPipelineEmpty(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	p := NewPipeline(r)
	tree, err := p.Run()
	if err != nil {
		t.Fatal(err)
	}
	if id := tree.Hash(); id != EmptyTreeId {
		t.Fatalf("%v", id)
	}
}

func TestPipelineAssert(t *testing.T) {
	r, tree := tmpTree(t)
	defer nukeRepo(r)

	treeOut, err := tree.Pipeline().AssertNotExist("foo").Run()
	if err != nil {
		t.Fatal(err)
	}
	if treeOut.Hash() != tree.Hash() {
		t.Fatalf("assertion changed output tree: %s -> %s\n", tree.Hash(), treeOut.Hash())
	}

	_, err = tree.Pipeline().AssertEq("foo", "bar").Run()
	if err == nil {
		t.Fatalf("wrong assertion did not trigger an error")
	}

	// Now create an entry and test the opposite
	tree, err = tree.Set("foo", "bar")
	if err != nil {
		t.Fatal(err)
	}

	_, err = tree.Pipeline().AssertNotExist("foo").Run()
	if err == nil {
		t.Fatalf("wrong assertion did not trigger an error")
	}

	treeOut, err = tree.Pipeline().AssertEq("foo", "bar").Run()
	if err != nil {
		t.Fatal(err)
	}
	if treeOut.Hash() != tree.Hash() {
		t.Fatalf("assertion changed output tree: %s -> %s\n", tree.Hash(), treeOut.Hash())
	}
	_, err = tree.Pipeline().AssertEq("foo", "WRONG VALUE").Run()
	if err == nil {
		t.Fatalf("wrong assertion did not trigger an error")
	}
}

func TestPipelineSet(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	tree, err := NewPipeline(r).Set("foo", "bar").Set("a/b/c/d", "hello world").Set("foo", "baz").Run()
	if err != nil {
		t.Fatal(err)
	}
	assert := tree.Pipeline().AssertEq("foo", "baz").AssertEq("a/b/c/d", "hello world")
	if _, err := assert.Run(); err != nil {
		t.Fatal(err)
	}
}

func TestPipelineAddTree(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	tree1, err := NewPipeline(r).Set("foo", "bar").Run()
	if err != nil {
		t.Fatal(err)
	}
	tree2, err := NewPipeline(r).Set("a/b/c/d", "hello world").Add("a", tree1, true).Run()
	if err != nil {
		t.Fatal(err)
	}
	assert := tree2.Pipeline().AssertEq("a/b/c/d", "hello world").AssertEq("a/foo", "bar")
	if _, err := assert.Run(); err != nil {
		t.Fatal(err)
	}
}

func TestPipelineAdd(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	foobar := NewPipeline(r).Set("foo", "bar")
	tree, err := NewPipeline(r).Set("hello", "world").Set("foo", "abc").Add("subdir", foobar, true).Run()
	if err != nil {
		t.Fatal(err)
	}
	assert := tree.Pipeline().AssertEq("hello", "world").AssertEq("subdir/foo", "bar").AssertEq("foo", "abc")
	if _, err := assert.Run(); err != nil {
		t.Fatal(err)
	}
}

func TestPipelineDelete(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	tree, err := NewPipeline(r).Set("hello", "world").Delete("hello").Run()
	if err != nil {
		t.Fatal(err)
	}
	assert := tree.Pipeline().AssertNotExist("hello")
	if _, err := assert.Run(); err != nil {
		t.Fatal(err)
	}
}

func TestPipelineScope(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	tree, err := NewPipeline(r).Set("a/b/c/d", "hello").Scope("a/b/c").Run()
	if err != nil {
		t.Fatal(err)
	}
	assert := tree.Pipeline().AssertEq("d", "hello")
	if _, err := assert.Run(); err != nil {
		t.Fatal(err)
	}
}

func TestPipelineDump(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	var buf bytes.Buffer
	NewPipeline(r).Set("foo", "bar").Dump(&buf).Delete("foo").Run()
	if dump := buf.String(); dump != "foo = bar\n" {
		t.Fatalf("%#v --> |%v|\n", dump)
	}
}

func TestPipelineOnRun(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)

	var called bool
	run := func(p *Pipeline) (*Tree, error) {
		called = true
		return p.Run()
	}
	p1 := NewPipeline(r)
	p2 := NewPipeline(r).OnRun(run)

	t1, err := p1.Run()
	if err != nil {
		t.Fatal(err)
	}
	t2, err := p2.Run()
	if err != nil {
		t.Fatal(err)
	}
	if t1.Hash() != t2.Hash() {
		t.Fatalf("%s != %s\n", t1.Hash(), t2.Hash())
	}
	if !called {
		t.Fatalf("run handler not called")
	}
}

func TestPipelineConcat(t *testing.T) {
	r, empty := tmpTree(t)
	defer nukeRepo(r)

	in, err := empty.Set("foo", "bar")
	if err != nil {
		t.Fatal(err)
	}

	step1 := NewPipeline(r).Add("/", in, false)
	step2 := NewPipeline(r).Set("hello", "world")
	p := concat(step1, step2)

	if _, err := p.AssertEq("foo", "bar").AssertEq("hello", "world").Run(); err != nil {
		t.Fatal(err)
	}
	out, err := p.Run()
	if err != nil {
		t.Fatal(err)
	}

	assert := out.Pipeline().AssertEq("foo", "bar").AssertEq("hello", "world")
	if _, err := assert.Run(); err != nil {
		t.Fatal(err)
	}
}

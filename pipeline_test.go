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

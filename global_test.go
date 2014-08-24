package libpack

import (
	"os"
	"testing"
)

func tmpGlobal(t *testing.T, ref string) *GlobalTree {
	if ref == "" {
		ref = "refs/libpack/test/global"
	}
	tmp := tmpdir(t)
	g, err := InitGlobal(tmp, ref)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func nukeGlobal(g *GlobalTree) {
	dir := g.db.Repo().Path()
	os.RemoveAll(dir)
}

func TestInitGlobal(t *testing.T) {
	g := tmpGlobal(t, "")
	defer nukeGlobal(g)
	db, _ := Open(g.db.Repo().Path(), "refs/whatever")
	db.Set("shykes/myapp/0.2", "hello world")
	db.Set("shykes/myapp/latest", "hello world")
	if err := g.LoadMount(&Mount{Dst: "mountpoint", Src: db.tree.Id()}); err != nil {
		t.Fatal(err)
	}
	assertNotExist(t, g, "/mountpoint/shykes/myapp/0.2")
	assertNotExist(t, g, "/mountpoint/shykes/myapp/latest")
	if err := g.Mount("/mountpoint"); err != nil {
		t.Fatal(err)
	}
	assertGet(t, g, "/mountpoint/shykes/myapp/0.2", "hello world")
	assertGet(t, g, "/mountpoint/shykes/myapp/latest", "hello world")
}

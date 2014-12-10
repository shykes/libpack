package libpack

import (
	"fmt"
	"os"
	"path"
	"strings"
	"testing"
)

var (
	// Scope values which should not actually change the scope
	nopScopes = []string{"", "/", "."}
)

func assertGet(t *testing.T, p *Pipeline, key, val string) {
	if v, err := p.Get(key); err != nil {
		fmt.Fprintf(os.Stderr, "--- db dump ---\n")
		p.Dump(os.Stderr).Run()
		fmt.Fprintf(os.Stderr, "--- end db dump ---\n")
		t.Fatalf("assert %v=%v db:%#v\n=> %#v", key, val, p, err)
	} else if v != val {
		fmt.Fprintf(os.Stderr, "--- db dump ---\n")
		p.Dump(os.Stderr).Run()
		fmt.Fprintf(os.Stderr, "--- end db dump ---\n")
		t.Fatalf("assert %v=%v db:%#v\n=> %v=%#v", key, val, p, key, v)
	}
}

// Assert that the specified key does not exist in db
func assertNotExist(t *testing.T, db *DB, key string) {
	if _, err := db.Get(key); err == nil {
		fmt.Fprintf(os.Stderr, "--- db dump ---\n")
		db.Dump(os.Stderr)
		fmt.Fprintf(os.Stderr, "--- end db dump ---\n")
		t.Fatalf("assert key %v doesn't exist db:%#v\n=> %v", key, db, err)
	}
}

func TestDBSetEmpty(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)

	db, err := r.DB("")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Set("foo", ""); err != nil {
		t.Fatal(err)
	}
}

func TestDBList(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	db, err := r.DB("")
	if err != nil {
		t.Fatal(err)
	}

	db.Set("foo", "bar")
	for _, rootpath := range []string{"", ".", "/", "////", "///."} {
		names, err := db.List(rootpath)
		if err != nil {
			t.Fatalf("%s: %v", rootpath, err)
		}
		if fmt.Sprintf("%v", names) != "[foo]" {
			t.Fatalf("List(%v) =  %#v", rootpath, names)
		}
	}
	for _, wrongpath := range []string{
		"does-not-exist",
		"sldhfsjkdfhkjsdfh",
		"a/b/c/d",
		"foo/sdfsdf",
	} {
		_, err := db.List(wrongpath)
		if err == nil {
			t.Fatalf("should fail: %s", wrongpath)
		}
		if !strings.Contains(err.Error(), "does not exist in the given tree") {
			t.Fatalf("wrong error: %v", err)
		}
	}
}

func TestDBSetGetSimple(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	db, err := r.DB("")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Set("foo", "bar"); err != nil {
		t.Fatal(err)
	}
	if key, err := db.Get("foo"); err != nil {
		t.Fatal(err)
	} else if key != "bar" {
		t.Fatalf("%#v", key)
	}
}

func TestDBSetGetMultiple(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	db, err := r.DB("")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Set("foo", "bar"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Set("ga", "bu"); err != nil {
		t.Fatal(err)
	}
	if key, err := db.Get("foo"); err != nil {
		t.Fatal(err)
	} else if key != "bar" {
		t.Fatalf("%#v", key)
	}
	if key, err := db.Get("ga"); err != nil {
		t.Fatal(err)
	} else if key != "bu" {
		t.Fatalf("%#v", key)
	}
}

func TestDBCommitConcurrentNoConflict(t *testing.T) {
	r1 := tmpRepo(t)
	defer nukeRepo(r1)

	db1, err := r1.DB("")
	if err != nil {
		t.Fatal(err)
	}

	r2, err := Init(r1.gr.Path(), false)
	if err != nil {
		t.Fatal(err)
	}

	db2, err := r2.DB(db1.Name())
	if err != nil {
		t.Fatal(err)
	}

	db1.Set("foo", "A")
	db2.Set("bar", "B")

	assertGet(t, db1.Query(), "foo", "A")
	assertGet(t, db2.Query(), "bar", "B")

	r3, _ := Init(r1.gr.Path(), false)
	db3, err := r3.DB(db1.Name())
	if err != nil {
		t.Fatal(err)
	}

	assertGet(t, db3.Query(), "foo", "A")
	assertGet(t, db3.Query(), "bar", "B")
}

func TestDBCommitConcurrentWithConflict(t *testing.T) {
	r1 := tmpRepo(t)
	defer nukeRepo(r1)

	db1, err := r1.DB("")
	if err != nil {
		t.Fatal(err)
	}

	r2, err := Init(r1.gr.Path(), false)
	if err != nil {
		t.Fatal(err)
	}
	db2, err := r2.DB(db1.Name())
	if err != nil {
		t.Fatal(err)
	}

	db1.Set("foo", "A")
	db2.Set("foo", "B")

	db1.Set("1", "written by 1")
	db1.Set("2", "written by 2")

	assertGet(t, db1.Query(), "foo", "A")
	assertGet(t, db2.Query(), "foo", "B")

	r3, _ := Init(r1.gr.Path(), false)
	db3, err := r3.DB(db1.Name())
	if err != nil {
		t.Fatal(err)
	}

	assertGet(t, db3.Query(), "foo", "B")
	assertGet(t, db3.Query(), "1", "written by 1")
	assertGet(t, db3.Query(), "2", "written by 2")
}

func TestDBSetCommitGet(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	db, err := r.DB("test")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Set("foo", "bar"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Set("ga", "bu"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Query().Set("ga", "added after commit").Run(); err != nil {
		t.Fatal(err)
	}

	// Re-open the repo
	r, err = Init(r.gr.Path(), false)
	if err != nil {
		t.Fatal(err)
	}

	db, err = r.DB("test")
	if err != nil {
		t.Fatal(err)
	}

	if val, err := db.Get("foo"); err != nil {
		t.Fatal(err)
	} else if val != "bar" {
		t.Fatalf("%#v", val)
	}
	if val, err := db.Get("ga"); err != nil {
		t.Fatal(err)
	} else if val != "bu" {
		t.Fatalf("%#v", val)
	}
}

func TestDBSetGetNested(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	db, err := r.DB("test")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Set("a/b/c/d/hello", "world"); err != nil {
		t.Fatal(err)
	}
	if key, err := db.Get("a/b/c/d/hello"); err != nil {
		t.Fatal(err)
	} else if key != "world" {
		t.Fatalf("%#v", key)
	}
}

func testSetGet(t *testing.T, refs []string, scopes []string, components ...[]string) {
	for _, ref := range refs {
		root := tmpRepo(t)
		defer nukeRepo(root)

		rootdb, err := root.DB(ref)
		if err != nil {
			t.Fatal(err)
		}

		for _, scope := range scopes {
			q := rootdb.Query().Scope(scope)
			if len(components) == 0 {
				return
			}
			if len(components) == 1 {
				for _, k := range components[0] {
					q = q.Set(k, "hello world")
				}
				for _, k := range components[0] {
					if v, err := q.Get(k); err != nil {
						t.Fatalf("Get('%s'): %v\n\troot=%#v\n\tscoped=%#v", k, err, rootdb, q)
					} else if v != "hello world" {
						t.Fatal(err)
					}
				}
				return
			}
			// len(components) >= 2
			first := make([]string, 0, len(components[0])*len(components[1]))
			for _, prefix := range components[0] {
				for _, suffix := range components[1] {
					first = append(first, path.Join(prefix, suffix))
				}
			}
			newComponents := append([][]string{first}, components[2:]...)
			testSetGet(t, []string{ref}, []string{scope}, newComponents...)
		}
	}
}

func TestDBSetGetNestedMultiple1(t *testing.T) {
	testSetGet(t,
		[]string{"refs/heads/test"},
		[]string{""},
		[]string{"foo"}, []string{"1", "2", "3", "4"}, []string{"/a/b/c/d/hello"},
	)
}

func TestDBSetGetNestedMultiple(t *testing.T) {
	testSetGet(t,
		[]string{"refs/heads/test"},
		[]string{""},
		[]string{"1", "2", "3", "4"}, []string{"/a/b/c/d/hello"},
	)
}

func TestDBSetGetNestedMultipleScoped(t *testing.T) {
	testSetGet(t,
		[]string{"refs/heads/test"},
		[]string{"0.1"},
		[]string{"1", "2", "3", "4"}, []string{"/a/b/c/d/hello"},
	)
}

func TestDBMkdir(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)
	db, err := r.DB("")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Mkdir("/"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Mkdir("something"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Mkdir("something"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Mkdir("foo/bar"); err != nil {
		t.Fatal(err)
	}
}

func TestDBDelete(t *testing.T) {
	r := tmpRepo(t)
	defer nukeRepo(r)

	db, err := r.DB("")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Set("test", "quux"); err != nil {
		t.Fatal(err)
	}
	str, err := db.Get("test")
	if err != nil {
		t.Fatal(err)
	}
	if str != "quux" {
		t.Fatal("Test value was not retrieved with Get")
	}
	if _, err := db.Delete("test"); err != nil {
		t.Fatal(err)
	}
	_, err = db.Get("test")
	if err == nil {
		t.Fatal("Test key did not get deleted after delete call")
	}
}

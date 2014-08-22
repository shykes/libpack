package libpack

import (
	"os"
	"testing"
)

func TestStackAddDB(t *testing.T) {
	tmp := tmpdir(t)
	defer os.RemoveAll(tmp)
	db, err := GitInit(tmp, "refs/heads/test", "")
	if err != nil {
		t.Fatal(err)
	}
	db2, err := GitInit(tmp, "refs/heads/test2", "")
	if err != nil {
		t.Fatal(err)
	}
	db2.Set("1/2/3/4", "hello")
	s := NewStack()
	s.SetRW(db)
	s.AddRO(db2)
	if v, err := s.Get("1/2/3/4"); err != nil {
		t.Fatal(err)
	} else if v != "hello" {
		t.Fatalf("%#v", v)
	}
	s.Set("foo", "bar")
	s.Commit("test 1")
	if v, err := s.Get("foo"); err != nil {
		t.Fatal(err)
	} else if v != "bar" {
		t.Fatalf("%#v", v)
	}
	if v, err := db.Get("foo"); err != nil {
		t.Fatal(err)
	} else if v != "bar" {
		t.Fatalf("%#v", v)
	}
}

func TestStackAddStack(t *testing.T) {
	s := NewStack()
	s2 := NewStack()
	s.SetRW(s2)
}

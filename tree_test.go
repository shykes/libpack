package main

import (
	"strings"
	"testing"
)

func TestUpdateTreeString(t *testing.T) {
	tree := make(Tree)
	tree.Update("foo", "bar")
	if tree["foo"] != "bar" {
		t.Fatalf("%#v", tree)
	}
}

func TestUpdateTree1level(t *testing.T) {
	keyVariations := []string{
		"foo/bar",
		"./foo/bar",
		"./foo/bar/",
		"foo///bar////",
		"foo/////bar",
		"/foo/bar",
		"////foo////bar/",
		"foo/bar////////",
	}
	for _, key := range keyVariations {
		tree := make(Tree)
		tree.Update(key, "hello")
		foo := tree["foo"].(Tree)
		bar := foo["bar"].(string)
		if bar != "hello" {
			t.Fatalf("%#v", tree)
		}
	}
}

func TestUpdateTree2levels(t *testing.T) {
	keyVariations := []string{
		"foo/bar/baz",
		"./foo/bar/baz",
		"./foo/bar/baz/",
		"foo///bar////baz/////",
		"foo/////bar//////baz",
		"/foo/bar/baz",
		"////foo////bar/baz/",
		"foo/bar////////baz//////",
	}
	for _, key := range keyVariations {
		tree := make(Tree)
		tree.Update(key, "hello world")
		tree.Update(strings.Replace(key, "baz", "second", 1), "hello again")
		foo := tree["foo"].(Tree)
		bar := foo["bar"].(Tree)
		if baz := bar["baz"].(string); baz != "hello world" {
			t.Fatalf("%#v", tree)
		}
		if second := bar["second"].(string); second != "hello again" {
			t.Fatalf("%#v", tree)
		}
	}
}

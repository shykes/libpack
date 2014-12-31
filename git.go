package libpack

import (
	"fmt"
	"io"
	"path"
	"regexp"
	"time"

	git "github.com/libgit2/git2go"
)

// Removes a key from the tree.
func treeDel(repo *git.Repository, tree *git.Tree, key string) (*git.Tree, error) {
	var err error

	key = TreePath(key)
	base, leaf := path.Split(key)

	if tree != nil {
		if tree, err = treeScope(repo, tree, base); err != nil {
			return nil, err
		}
	}

	builder, err := repo.TreeBuilderFromTree(tree)
	if err != nil {
		return nil, err
	}

	if err := builder.Remove(leaf); err != nil {
		return nil, err
	}

	treeId, err := builder.Write()
	if err != nil {
		return nil, err
	}

	newTree, err := lookupTree(repo, treeId)
	if err != nil {
		return nil, err
	}

	return newTree, err
}

// treeAdd creates a new Git tree by adding a new object
// to it at the specified path.
// Intermediary subtrees are created as needed.
// If an object already exists at key or any intermediary path,
// it is overwritten.
//
// - If merge is true, new trees are merged into existing ones at the
// file granularity (similar to 'cp -R').
// - If it is false, existing trees are completely shadowed (similar to 'mount')
//
// Since git trees are immutable, base is not modified. The new
// tree is returned.
// If an error is encountered, intermediary objects may be left
// behind in the git repository. It is the caller's responsibility
// to perform garbage collection, if any.
// FIXME: manage garbage collection, or provide a list of created
// objects.
func treeAdd(repo *git.Repository, tree *git.Tree, key string, valueId *git.Oid, merge bool) (t *git.Tree, err error) {
	/*
	** // Primitive but convenient tracing for debugging recursive calls to treeAdd.
	** // Uncomment this block for debug output.
	**
	** var callString string
	** if tree != nil {
	** 		callString = fmt.Sprintf("   treeAdd %v:\t\t%s\t\t\t= %v", tree.Id(), key, valueId)
	** 	} else {
	** 		callString = fmt.Sprintf("   treeAdd %v:\t\t%s\t\t\t= %v", tree, key, valueId)
	** 	}
	** 	fmt.Printf("   %s\n", callString)
	** 	defer func() {
	** 		if t != nil {
	** 			fmt.Printf("-> %s => %v\n", callString, t.Id())
	** 		} else {
	** 			fmt.Printf("-> %s => %v\n", callString, err)
	** 		}
	** 	}()
	 */
	if valueId == nil {
		return tree, nil
	}
	key = TreePath(key)
	base, leaf := path.Split(key)
	o, err := repo.Lookup(valueId)
	if err != nil {
		return nil, err
	}
	var builder *git.TreeBuilder
	if tree == nil {
		builder, err = repo.TreeBuilder()
		if err != nil {
			return nil, err
		}
	} else {
		builder, err = repo.TreeBuilderFromTree(tree)
		if err != nil {
			return nil, err
		}
	}
	defer builder.Free()
	// The specified path has only 1 component (the "leaf")
	if base == "" || base == "/" {
		// If val is a string, set it and we're done.
		// Any old value is overwritten.
		if _, isBlob := o.(*git.Blob); isBlob {
			if err := builder.Insert(leaf, valueId, 0100644); err != nil {
				return nil, err
			}
			newTreeId, err := builder.Write()
			if err != nil {
				return nil, err
			}
			newTree, err := lookupTree(repo, newTreeId)
			if err != nil {
				return nil, err
			}
			return newTree, nil
		}
		// If val is not a string, it must be a subtree.
		// Return an error if it's any other type than Tree.
		oTree, ok := o.(*git.Tree)
		if !ok {
			return nil, fmt.Errorf("value must be a blob or subtree")
		}
		var subTree *git.Tree
		var oldSubTree *git.Tree
		if tree != nil {
			oldSubTree, err = treeScope(repo, tree, leaf)
			// FIXME: distinguish "no such key" error (which
			// FIXME: distinguish a non-existing previous tree (continue with oldTree==nil)
			// from other errors (abort and return an error)
			if err == nil {
				defer oldSubTree.Free()
			}
		}
		// If that subtree already exists, merge the new one in.
		if merge && oldSubTree != nil {
			subTree = oldSubTree
			for i := uint64(0); i < oTree.EntryCount(); i++ {
				var err error
				e := oTree.EntryByIndex(i)
				subTree, err = treeAdd(repo, subTree, e.Name, e.Id, merge)
				if err != nil {
					return nil, err
				}
			}
		} else {
			subTree = oTree
		}
		// If the key is /, we're replacing the current tree
		if key == "/" {
			return subTree, nil
		}
		// Otherwise we're inserting into the current tree
		if err := builder.Insert(leaf, subTree.Id(), 040000); err != nil {
			return nil, err
		}
		newTreeId, err := builder.Write()
		if err != nil {
			return nil, err
		}
		newTree, err := lookupTree(repo, newTreeId)
		if err != nil {
			return nil, err
		}
		return newTree, nil
	}
	subtree, err := treeAdd(repo, nil, leaf, valueId, merge)
	if err != nil {
		return nil, err
	}
	return treeAdd(repo, tree, base, subtree.Id(), merge)
}

func treeWalk(r *git.Repository, t *git.Tree, key string, h func(string, git.Object) error) error {
	if t == nil {
		return fmt.Errorf("no tree to walk")
	}
	subtree, err := treeScope(r, t, key)
	if err != nil {
		return err
	}
	var handlerErr error
	err = subtree.Walk(func(parent string, e *git.TreeEntry) int {
		obj, err := r.Lookup(e.Id)
		if err != nil {
			handlerErr = err
			return -1
		}
		if err := h(path.Join(parent, e.Name), obj); err != nil {
			handlerErr = err
			return -1
		}
		obj.Free()
		return 0
	})
	if handlerErr != nil {
		return handlerErr
	}
	if err != nil {
		return err
	}
	return nil
}

func treeDump(r *git.Repository, t *git.Tree, key string, dst io.Writer) error {
	return treeWalk(r, t, key, func(key string, obj git.Object) error {
		if _, isTree := obj.(*git.Tree); isTree {
			fmt.Fprintf(dst, "%s/\n", key)
		} else if blob, isBlob := obj.(*git.Blob); isBlob {
			fmt.Fprintf(dst, "%s = %s\n", key, blob.Contents())
		}
		return nil
	})
}

func treeScope(repo *git.Repository, tree *git.Tree, name string) (*git.Tree, error) {
	if tree == nil {
		return nil, fmt.Errorf("tree undefined")
	}
	name = TreePath(name)
	if name == "/" {
		// Allocate a new Tree object so that the caller
		// can always call Free() on the result
		return lookupTree(repo, tree.Id())
	}
	entry, err := tree.EntryByPath(name)
	if err != nil {
		return nil, err
	}
	return lookupTree(repo, entry.Id)
}

func gitCommitFromRef(r *git.Repository, ref string) (*git.Commit, error) {
	tip, err := r.LookupReference(ref)
	if err != nil {
		return nil, err
	}
	return lookupCommit(r, tip.Target())
}

// commitToRef creates a new commit object from the specified parent commit, content tree,
// message and repository.
// It updates the value of `refname` to point to the new commit, or returns an error if that
// fails.
func commitToRef(r *git.Repository, tree *git.Tree, parent *git.Commit, refname, msg string) (*git.Commit, error) {
	// Retry loop in case of conflict
	// FIXME: use a custom inter-process lock as a first attempt for performance
	var (
		needMerge bool
		tmpCommit *git.Commit
	)
	for {
		if !needMerge {
			// Create simple commit
			commit, err := mkCommit(r, refname, msg, tree, parent)
			if isGitConcurrencyErr(err) {
				needMerge = true
				continue
			}
			return commit, err
		} else {
			if tmpCommit == nil {
				var err error
				// Create a temporary intermediary commit, to pass to MergeCommits
				// NOTE: this commit will not be part of the final history.
				tmpCommit, err = mkCommit(r, "", msg, tree, parent)
				if err != nil {
					return nil, err
				}
				defer tmpCommit.Free()
			}
			// Lookup tip from ref
			tip := lookupTip(r, refname)
			if tip == nil {
				// Ref may have been deleted after previous merge error
				needMerge = false
				continue
			}

			// Merge simple commit with the tip
			opts, err := git.DefaultMergeOptions()
			if err != nil {
				return nil, err
			}
			idx, err := r.MergeCommits(tmpCommit, tip, &opts)
			if err != nil {
				return nil, err
			}
			conflicts, err := idx.ConflictIterator()
			if err != nil {
				return nil, err
			}
			defer conflicts.Free()
			for {
				c, err := conflicts.Next()
				if isGitIterOver(err) {
					break
				} else if err != nil {
					return nil, err
				}
				if c.Our != nil {
					idx.RemoveConflict(c.Our.Path)
					if err := idx.Add(c.Our); err != nil {
						return nil, fmt.Errorf("error resolving merge conflict for '%s': %v", c.Our.Path, err)
					}
				}
			}
			mergedId, err := idx.WriteTreeTo(r)
			if err != nil {
				return nil, fmt.Errorf("WriteTree: %v", err)
			}
			mergedTree, err := lookupTree(r, mergedId)
			if err != nil {
				return nil, err
			}
			// Create new commit from merged tree (discarding simple commit)
			commit, err := mkCommit(r, refname, msg, mergedTree, parent, tip)
			if isGitConcurrencyErr(err) {
				// FIXME: enforce a maximum number of retries to avoid infinite loops
				continue
			}
			return commit, err
		}
	}
	return nil, fmt.Errorf("too many failed merge attempts, giving up")
}

func mkCommit(r *git.Repository, refname string, msg string, tree *git.Tree, parent *git.Commit, extraParents ...*git.Commit) (*git.Commit, error) {
	var parents []*git.Commit
	if parent != nil {
		parents = append(parents, parent)
	}
	if len(extraParents) > 0 {
		parents = append(parents, extraParents...)
	}
	id, err := r.CreateCommit(
		refname,
		&git.Signature{"libpack", "libpack", time.Now()}, // author
		&git.Signature{"libpack", "libpack", time.Now()}, // committer
		msg,
		tree, // git tree to commit
		parents...,
	)
	if err != nil {
		return nil, err
	}
	return lookupCommit(r, id)
}

func isGitConcurrencyErr(err error) bool {
	gitErr, ok := err.(*git.GitError)
	if !ok {
		return false
	}
	return gitErr.Class == 11 && gitErr.Code == -15
}

func isGitIterOver(err error) bool {
	gitErr, ok := err.(*git.GitError)
	if !ok {
		return false
	}
	return gitErr.Code == git.ErrIterOver
}

// IsGitNoRefErr returns a boolean indicating whether the error is known
// to report that a git reference does not exist.
func isGitNoRefErr(err error) bool {
	// FIXME: this error does not seem to match git.GitError, so we
	// rely on the text string to detect it. This is not ideal, better
	// suggestions are welcome.
	if err == nil {
		return false
	}
	matched, err := regexp.MatchString("^Reference '[^']*' not found$", err.Error())
	if err != nil {
		return false
	}
	return matched
}

func lookupTree(r *git.Repository, id *git.Oid) (*git.Tree, error) {
	obj, err := r.Lookup(id)
	if err != nil {
		return nil, err
	}
	if tree, ok := obj.(*git.Tree); ok {
		return tree, nil
	}
	return nil, fmt.Errorf("hash %v exist but is not a tree", id)
}

// lookupBlob looks up an object at hash `id` in `repo`, and returns
// it as a git blob. If the object is not a blob, an error is returned.
func lookupBlob(r *git.Repository, id *git.Oid) (*git.Blob, error) {
	obj, err := r.Lookup(id)
	if err != nil {
		return nil, err
	}
	if blob, ok := obj.(*git.Blob); ok {
		return blob, nil
	}
	return nil, fmt.Errorf("hash %v exist but is not a blob", id)
}

// lookupTip looks up the object referenced by refname, and returns it
// as a Commit object. If the reference does not exist, or if object is
// not a commit, nil is returned. Other errors cannot be detected.
func lookupTip(r *git.Repository, refname string) *git.Commit {
	ref, err := r.LookupReference(refname)
	if err != nil {
		return nil
	}
	commit, err := lookupCommit(r, ref.Target())
	if err != nil {
		return nil
	}
	return commit
}

// lookupCommit looks up an object at hash `id` in `repo`, and returns
// it as a git commit. If the object is not a commit, an error is returned.
func lookupCommit(r *git.Repository, id *git.Oid) (*git.Commit, error) {
	obj, err := r.Lookup(id)
	if err != nil {
		return nil, err
	}
	if commit, ok := obj.(*git.Commit); ok {
		return commit, nil
	}
	return nil, fmt.Errorf("hash %v exist but is not a commit", id)
}

// emptyTree creates an empty Git tree and returns its ID
// (the ID will always be the same)
func emptyTree(repo *git.Repository) (*git.Oid, error) {
	builder, err := repo.TreeBuilder()
	if err != nil {
		return nil, err
	}
	return builder.Write()
}

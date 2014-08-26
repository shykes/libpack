package libpack

import (
	"fmt"
	"path"

	git "github.com/libgit2/git2go"
)

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
			oldSubTree, err = lookupSubtree(repo, tree, leaf)
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

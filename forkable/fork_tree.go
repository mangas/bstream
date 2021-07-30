package forkable

import (
	"fmt"
	"sort"
)

type ChainList struct {
	Chains [][]string
}

func (l *ChainList) LongestChain() []string {
	longestID := -1
	longestLen := 0
	for i, chain := range l.Chains {
		if len(chain) > longestLen {
			longestLen = len(chain)
			longestID = i
		}
	}

	if len(l.Chains) > 0 {
		return l.Chains[longestID]
	}

	return nil
}

type Node struct {
	id       string
	children []*Node
}

func newNode(id string) *Node {
	return &Node{
		id: id,
	}
}

func (n *Node) growBranches(db *ForkDB) {
	children := db.findChildren(n.id)

	for _, childID := range children {
		node := newNode(childID)
		node.growBranches(db)
		n.children = append(n.children, node)
	}
}
func (n *Node) chains(current []string, out *ChainList) {
	current = append(current, n.id)
	if len(n.children) == 0 { //reach the leaf
		out.Chains = append(out.Chains, current)
		return
	}

	for _, child := range n.children {
		c := make([]string, len(current))
		copy(c, current)
		child.chains(c, out)
	}
}

//ForkDB addons
func (db *ForkDB) BuildTree() (*Node, error) {
	db.linksLock.Lock()
	defer db.linksLock.Unlock()

	root, err := db.root()
	if err != nil {
		return nil, err
	}
	return db.buildTreeWithID(root), nil
}

func (n *Node) Chains() (*ChainList, error) {
	chains := &ChainList{
		Chains: [][]string{},
	}
	n.chains(nil, chains)

	return chains, nil
}

func (db *ForkDB) BuildTreeWithID(root string) *Node {
	db.linksLock.Lock()
	defer db.linksLock.Unlock()

	return db.buildTreeWithID(root)
}
func (db *ForkDB) buildTreeWithID(root string) *Node {
	rootNode := newNode(root)
	rootNode.growBranches(db)
	return rootNode
}

func (db *ForkDB) findChildren(parentID string) []string {
	var children []string
	for id, prevID := range db.links {
		if prevID == parentID {
			children = append(children, id)
		}
	}
	sort.Strings(children)
	return children
}
func (db *ForkDB) roots() []string {
	var roots []string
	for blockID, prevID := range db.links {
		if _, found := db.links[prevID]; !found {
			roots = append(roots, blockID)
		}
	}
	sort.Strings(roots)
	return roots
}

func (db *ForkDB) root() (string, error) {
	if len(db.links) == 0 {
		return "", fmt.Errorf("no link")
	}
	roots := db.roots()

	if len(roots) > 1 {
		return "", fmt.Errorf("multiple root found: %d", len(roots))
	}
	return roots[0], nil
}

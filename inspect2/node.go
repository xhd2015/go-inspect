package inspect

import (
	"fmt"
	"go/ast"
	"go/token"
)

type rewriteVisitor struct {
	parent      *rewriteVisitor // for debug
	rangeNode   ast.Node
	rewrite     func(node ast.Node, getNodeText func(start token.Pos, end token.Pos) []byte) ([]byte, bool)
	getNodeText func(start token.Pos, end token.Pos) []byte

	children       []*rewriteVisitor
	rangeRewrite   []byte
	rangeRewriteOK bool
	// childRewrite map[ast.Node][]byte
}

// during walk, c is for node
func (c *rewriteVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil { // end of current visiting
		return nil
	}
	// debug check
	if c.parent != nil && c.parent.rangeNode != nil {
		if node.Pos() < c.parent.rangeNode.Pos() {
			panic(fmt.Errorf("node begin lower than parent's:%d < %d, %v", node.Pos(), c.parent.rangeNode.Pos(), node))
		}
		if node.End() > c.parent.rangeNode.End() {
			panic(fmt.Errorf("node end bigger than parent's:%d > %d %v", node.End(), c.parent.rangeNode.End(), node))
		}
	}

	// child
	child := &rewriteVisitor{
		parent:      c,
		rangeNode:   node,
		rewrite:     c.rewrite,
		getNodeText: c.getNodeText,
	}
	c.children = append(c.children, child)
	if c.rewrite != nil {
		child.rangeRewrite, child.rangeRewriteOK = c.rewrite(node, c.getNodeText)
		if child.rangeRewriteOK {
			// this node gets written
			// do not traverse its children any more.
			return nil
		}
	}

	return child
}

func (c *rewriteVisitor) join(depth int, hook func(ast.Node, []byte) []byte) []byte {
	if c.rangeRewriteOK {
		return hook(c.rangeNode, c.rangeRewrite)
	}
	var res []byte

	off := c.rangeNode.Pos()
	// sort.Slice(c.children, func(i, j int) bool {
	// 	return c.children[i].rangeNode.Pos() < c.children[j].rangeNode.Pos()
	// })
	// check (always correct)
	// for i, ch := range c.children {
	// 	if i == 0 {
	// 		continue
	// 	}
	// 	last := c.children[i-1]
	// 	// begin of this node should be bigger than last node's end
	// 	if ch.rangeNode.Pos() < last.rangeNode.End() {
	// 		panic(fmt.Errorf("node overlap:%d", i))
	// 	}
	// }

	for _, ch := range c.children {
		n := ch.rangeNode
		nstart := n.Pos()
		nend := n.End()
		// missing slots
		// off->start
		res = append(res, c.getNodeText(off, nstart)...)

		// start->end
		res = append(res, ch.join(depth+1, hook)...)

		// update off
		off = nend
	}

	// process trailing
	if off != token.NoPos {
		res = append(res, c.getNodeText(off, c.rangeNode.End())...)
	}
	return hook(c.rangeNode, res)
}

type AstNodeRewritter = func(node ast.Node, getNodeText func(start token.Pos, end token.Pos) []byte) ([]byte, bool)

// joining them together makes a complete statement, in string format.
func RewriteAstNodeText(node ast.Node, getNodeText func(start token.Pos, end token.Pos) []byte, rewrite AstNodeRewritter) []byte {
	return RewriteAstNodeTextHooked(node, getNodeText, rewrite, nil)
}

func RewriteAstNodeTextHooked(node ast.Node, getNodeText func(start token.Pos, end token.Pos) []byte, rewrite AstNodeRewritter, hook func(node ast.Node, c []byte) []byte) []byte {
	// traverse to get all rewrite info
	root := &rewriteVisitor{
		rewrite:     rewrite,
		getNodeText: getNodeText,
	}
	ast.Walk(root, node)
	if hook == nil {
		hook = func(node ast.Node, c []byte) []byte {
			return c
		}
	}

	// parent is responsible for fill in uncovered slots
	return root.children[0].join(0, hook)
}

// like filter, first hook gets firstly executed
func CombineHooks(hooks ...func(node ast.Node, c []byte) []byte) func(node ast.Node, c []byte) []byte {
	cur := func(node ast.Node, c []byte) []byte {
		return c
	}
	for _, _hook := range hooks {
		if _hook == nil {
			continue
		}
		hook := _hook
		last := cur
		cur = func(node ast.Node, c []byte) []byte {
			return hook(node, last(node, c))
		}
	}
	return cur
}

// like filter, first hook gets firstly executed
func CombineHooksStr(hooks ...func(node ast.Node, c string) string) func(node ast.Node, c []byte) []byte {
	cur := func(node ast.Node, c []byte) []byte {
		return c
	}
	for _, _hook := range hooks {
		if _hook == nil {
			continue
		}
		hook := _hook
		last := cur
		cur = func(node ast.Node, c []byte) []byte {
			// TODO: for large node, be more memory-efficient
			return []byte(hook(node, string(last(node, c))))
		}
	}
	return cur
}

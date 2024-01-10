package edit

import "go/token"

// Edit represents
type Edit interface {
	// Insert insert after the given position
	Insert(start token.Pos, content string)
	Delete(start token.Pos, end token.Pos)
	Replace(start token.Pos, end token.Pos, newContent string)

	String() string
}

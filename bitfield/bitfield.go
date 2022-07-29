package bitfield

// A Bitfield represents the pieces that a peer has
// the reason why we need this type is simple- Go doesn't let you define
// methods on types that aren't in the same package. I ran into this firsthand
// the solution is to create your own type for a common type like []byte
// so that you can write methods for it. Also I had a circular include
// problem so I couldn't do it in the same file as the type definition, in case you are wondering
type Bitfield []byte

// figure out whether or not a peer has a piece that the client needs,
// based on the "bitfield" field that the client has
// the ith BIT of the bitfield is going to tell you whether or not this peer has the ith piece
func (clientPieces Bitfield) HasPiece(requestedPieceIdx int) bool {
	// since its 8 bits per byte, first calculate
	// which byte it belongs to, then which bit its offset
	byteNo := requestedPieceIdx / 8
	byteNoOffset := requestedPieceIdx % 8

	// its 7- since if we want the first (leftmost) bit we'd have to do (>> 7)
	if clientPieces[byteNo]>>uint8(7-byteNoOffset) == 1 {
		// if 1, then yes it owns this piece
		return true
	} else {
		return false
	}
}

// kinda same as above, except we are setting it to 1 this time
func (b Bitfield) SetPiece(index int) {
	byteNo := index / 8
	byteNoOffset := index % 8
	b[byteNo] |= 1 << uint8(7-byteNoOffset)
}

package message

import (
	"encoding/binary"
	"fmt"
	"io"
	"main/bitfield"
)

// all message ID types in BitTorrent specification
// the message ID is always the 5th bit in the response from a peer
// using this bit we can figure out which kind of message it is
// also make sure its all capital letters so they get EXPORTED
const (
	Choke         uint8 = 0
	Unchoke       uint8 = 1
	Interested    uint8 = 2
	Notinterested uint8 = 3
	Have          uint8 = 4
	Bitfield      uint8 = 5
	Request       uint8 = 6
	Piece         uint8 = 7
	Cancel        uint8 = 8
	Port          uint8 = 9
)

// this struct represents a message sent back to us from the peer
type Message struct {
	Length  uint32 // length in bytes of the payload + id (aka 1 byte) (not including the 4 bytes for length itself)
	ID      uint8  // the ID tells us what kind of message it is
	Payload []byte // the actual contents of the message
}

// helper function to send a Message struct to an []byte thats ready for sending
func (m *Message) MessageToByteSlice() []byte {
	if m == nil {
		return make([]byte, 4)
	}

	// length = 1 (for id) + len(payload)
	length := uint32(1 + len(m.Payload))
	// message is length+4 since length is 4 bytes
	ret := make([]byte, 4+length)

	binary.BigEndian.PutUint32(ret[0:4], length)

	// copy ID in
	ret[4] = byte(m.ID)

	copy(ret[5:], m.Payload)
	return ret
}

// a general read function to parse stuff from the peer
func Read(r io.Reader) (*Message, error) {
	pstrlen := make([]byte, 4)
	_, err := io.ReadFull(r, pstrlen)
	if err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(pstrlen)

	// keep-alive message. Gotta filter for these else the [1:] slice below will index out of bounds
	if length == 0 {
		return nil, nil
	}

	restOfMessage := make([]byte, length)
	_, err = io.ReadFull(r, restOfMessage)
	if err != nil {
		return nil, err
	}
	ret := Message{
		Length:  length,
		ID:      restOfMessage[0],
		Payload: restOfMessage[1:],
	}
	return &ret, nil
}

// this is RETURNED to us from the peer!!!!
// piece: <len=0009+X><id=7><index><begin><block>
// The piece message is variable length, where X is the length of the block.
// The payload contains the following information:

// index: integer specifying the zero-based piece index
// begin: integer specifying the zero-based byte offset within the piece
// block: block of data, which is a subset of the piece specified by index.
func ParsePiece(pg bitfield.Bitfield, index int, m *Message) (int, error) {
	actualindex := int(binary.BigEndian.Uint32(m.Payload[0:4]))
	begin := int(binary.BigEndian.Uint32(m.Payload[4:8]))
	block := m.Payload[8:]

	if actualindex != index {
		return 0, fmt.Errorf("The index for this piece doesn't match the index we want")
	}

	if begin > len(pg) {
		return 0, fmt.Errorf("The peer tried to return a begin value that was larger than our piece size")
	}

	if begin+len(block) > len(pg) {
		return 0, fmt.Errorf("The peer's begin + data size is larger than our piece buffer size")
	}

	// this is the key- copy the piece contents returned from the peer into our struct
	copy(pg[begin:], block)
	return len(block), nil
}

// <len=0005><id=4><piece index>
// so payload is just the piece index that the peer has
func ParseHave(m *Message) int {
	return int(binary.BigEndian.Uint32(m.Payload[:]))
}

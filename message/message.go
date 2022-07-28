package message

import "encoding/binary"

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

// helper function to send a message struct to an []byte thats ready for sending
func (m *Message) MessageToByteSlice() []byte {
	// the total length of the message is what is in
	// the length field + 4 (for the length field itself)
	ret := make([]byte, 4+m.Length)

	binary.BigEndian.PutUint32(ret[0:4], uint32(1+len(m.Payload)))

	ret[5] = byte(m.ID)

	copy(ret[5:], m.Payload)
	return ret
}

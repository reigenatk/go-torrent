package client

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"main/bitfield"
	"main/message"
	"main/peers"
	"net"
	"time"
)

type Client struct {
	Conn net.Conn
	// has the peer choked our client?
	Choked bool
	// which pieces does this peer own?
	Bitfield bitfield.Bitfield
	peer     peers.Peer
	peerID   [20]byte
	infoHash [20]byte
}

// message format is bitfield: <len=0001+X><id=5><bitfield>
func receiveBitfieldMessage(conn net.Conn) (*message.Message, error) {
	// do deadline thing
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	defer conn.SetDeadline(time.Time{}) // Disable the deadline

	lengthOfMsg := make([]byte, 4)
	_, err := io.ReadFull(conn, lengthOfMsg)
	if err != nil {
		return nil, err
	}

	// "The length prefix is a four byte big-endian value."
	// The message ID is a single decimal byte. The payload is message dependent.
	msgLen := binary.BigEndian.Uint32(lengthOfMsg)
	if msgLen == 0 {
		err := fmt.Errorf("expected bitfield message, got keepalive instead")
		return nil, err
	}

	// read in the bitfield (remember that first byte is the ID, 5)
	bitFieldBuf := make([]byte, msgLen)
	_, err = io.ReadFull(conn, bitFieldBuf)
	if err != nil {
		return nil, err
	}

	if bitFieldBuf[0] != message.Bitfield {
		err := fmt.Errorf("expected bitfield message, got message with id %d instead", bitFieldBuf[0])
		return nil, err
	}

	// make a message struct to return
	msg := message.Message{
		Length:  msgLen,
		ID:      uint8(bitFieldBuf[0]),
		Payload: bitFieldBuf[1:],
	}
	return &msg, nil
}

// this actually forms the connection using net.Dial, and outputs a net.Conn variable
// and puts the connection into a Client struct for easy use later
func New(peer peers.Peer, peerID [20]byte, infoHash [20]byte, numPieces int) (*Client, error) {
	// we could just do net.Dial here but its better to use a timeout with this connection
	// therefore we choose to use net.DialTimeout instead
	// conn, err := net.Dial("tcp", peer.String())

	// lets use a 10 second timeout
	conn, err := net.DialTimeout("tcp", peer.String(), time.Second*10)
	if err != nil {
		return nil, err
	}

	// setup and perform handshake on this peer
	err = performPeerHandshake(conn, peerID, infoHash)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// receive the bitfield message that tells us what pieces this particular peer owns
	piecesOwned, err := receiveBitfieldMessage(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// A bitfield of the wrong length is considered an error. Clients should drop the
	// connection if they receive bitfields that are not of the correct size, or
	// if the bitfield has any of the spare bits set.
	// aka bitfield length is not equal to number of pieces, then terminate connection
	if len(piecesOwned.Payload) != numPieces {
		conn.Close()
		err = fmt.Errorf("Number of pieces is not equal to len of bitfield from peer return message")
		return nil, err
	}

	ret := Client{
		Conn:     conn,
		Choked:   true,
		Bitfield: piecesOwned.Payload,
		peer:     peer,
		peerID:   peerID,
		infoHash: infoHash,
	}
	return &ret, nil
}

// handshake format goes pstrlen, pstr, reserved, infohash, peerid
func performPeerHandshake(conn net.Conn, peerID [20]byte, infoHash [20]byte) error {
	// idk exactly why we need this, didnt we do DialTimeout on conn already?
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	defer conn.SetDeadline(time.Time{})

	// we basically have to transform handshake info into a single []byte
	pstrlen := 19
	pstr := "BitTorrent protocol"
	var reserved [8]byte

	// we can hardcode this slice's capacity as 49+len(pstr). Check the wiki for more info
	handshakeBuf := make([]byte, 49+len(pstr))
	handshakeBuf[0] = byte(pstrlen)
	cur := 1

	// copy supports strings as second arg
	cur += copy(handshakeBuf[cur:], pstr)
	cur += copy(handshakeBuf[cur:], reserved[:])
	cur += copy(handshakeBuf[cur:], infoHash[:])
	cur += copy(handshakeBuf[cur:], peerID[:])

	// send the byte slice into the connection...
	_, err := conn.Write(handshakeBuf)
	if err != nil {
		return err
	}

	// read the response from peer (should be exact same as the one we sent to peer)
	// using io.ReadFull, not io.ReadAll since that doesnt give us control over length
	// also IDK why we cant just read the whole response in at once but OK
	firstByte := make([]byte, 1)
	_, err = io.ReadFull(conn, firstByte)
	if err != nil {
		return err
	}
	pstrlenResponse := int(firstByte[0])
	if pstrlenResponse == 0 {
		err := fmt.Errorf("peer handshake failed, first byte (pstrlen) was %d", pstrlenResponse)
		return err
	}

	// read in the rest of the peer handshake response
	restOfResponse := make([]byte, 48+len(pstr))
	io.ReadFull(conn, restOfResponse)
	if err != nil {
		return err
	}
	// check that the peerIDs and infoHashes match
	if bytes.Compare(infoHash[:], restOfResponse[len(pstr)+8:len(pstr)+8+20]) != 0 {
		err := fmt.Errorf("peer handshake failed, infoHashes don't match")
		return err
	}
	if bytes.Compare(peerID[:], restOfResponse[len(pstr)+28:len(pstr)+28+20]) != 0 {
		err := fmt.Errorf("peer handshake failed, peerIDs don't match")
		return err
	}

	// otherwise we are happy. We've made a handshake, peer response was correct, and
	// now we can start transferring actual data
	return nil
}

// two quick functions to help us send a unchoke and interested message to the peer
// these are invoked in startDownloadPiece right after getting the bitmap of piece availability
func (client *Client) UnchokePeer() error {
	msg := message.Message{
		ID: message.Unchoke,
	}
	msgInBytes := msg.MessageToByteSlice()
	// send the byte slice into the connection...
	_, err := client.Conn.Write(msgInBytes)
	return err
}

func (client *Client) SendInterestedPeer() error {
	msg := message.Message{
		ID: message.Interested,
	}
	msgInBytes := msg.MessageToByteSlice()
	// send the byte slice into the connection...
	_, err := client.Conn.Write(msgInBytes)
	return err
}

func (client *Client) SendUnInterestedPeer() error {
	msg := message.Message{
		ID: message.Notinterested,
	}
	msgInBytes := msg.MessageToByteSlice()
	// send the byte slice into the connection...
	_, err := client.Conn.Write(msgInBytes)
	return err
}
func (client *Client) SendChoke() error {
	msg := message.Message{
		ID: message.Choke,
	}
	msgInBytes := msg.MessageToByteSlice()
	// send the byte slice into the connection...
	_, err := client.Conn.Write(msgInBytes)
	return err
}

// piece: <len=0009+X><id=7><index><begin><block>
// The piece message is variable length, where X is the length of the block. The payload contains the following information:

// index: integer specifying the zero-based piece index
// begin: integer specifying the zero-based byte offset within the piece
// block: block of data, which is a subset of the piece specified by index.

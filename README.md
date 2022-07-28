The torrent works as follows:

1. Decode the bencoded .torrent file to figure out the address of the tracker
2. Create the right URL which will visit the tracker announce page, making sure to have the right query parameters as specified by [bittorrent specification](https://wiki.theory.org/BitTorrentSpecification)
3. Make the request to the announce page, get a bencoded response back
4. Decode the response again, and get the list of peers.
5. Connect to each peer and do a handshake with each one (start x goroutines where x is the number of peers)
6. Grab the bitfield from the peer's response to the handshake. Also check that the handshake response contains the same peerID and infoHash. The bitfield tells us which pieces this particular peer owns
7. Create a []byte representing the handshake format, send it
8. Peer should send the exact same thing back, if not then sever the connection
9. Download all the pieces from the peers

Here's [another](http://dandylife.net/docs/BitTorrent-Protocol.pdf) useful reference

Some terminology:

The **tracker** is a service usually reachable over HTTP, also in the announce field of the `.torrent` file. You visit it first, and it tells you a list of the **peers**. A **peer** is an IP/port combination that is available over the internet, that talks to the tracker periodically, and is also going to give you certain chunks of the file you desire. 

A **piece** (also called a block) is simply a fragment of the entire file that we want to torrent. The philosophy behind torrent is that instead of getting the entire file from one source, we split the file up into many pieces and then we connect to multiple sources, each of which gives us a piece of the whole file. Then we stitch together the pieces and get the whole file. This is arguably better because it reduces bandwith compared to the first approach, since I don't need to spend minutes, maybe even hours if its a big file, connected to the same server, which may be trying to service many other requests at the same time. Also, torrenting may arguably be faster since I can open up many connections at once. Torrenting is kind of like multithreaded downloads, if you think about it.

There are also two key ideas in BitTorrent, **choke** and **interested**. Basically choke means, if you send me a message, will I process it? And interested means, if I unchoke you, will I begin to request blocks from you? So basically:

`A block is downloaded by the client when the client is interested in a peer, and that peer is not choking the client. A block is uploaded by a client when the client is not choking a peer, and that peer is interested in the client.`

It's a little confusing but if still confused check out the spec. The most important fact is that the client starts out choking and uninterested in the peer, so before we request any blocks from the peer (after the handshake but before the first block request), we need to send an **unchoke** and **interested** message to the peer.
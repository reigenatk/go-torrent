# go-torrent
I decided to learn some Go, seeing as it is one of the most popular languages [fifth in the most recent GitHub surevy!](https://madnight.github.io/githut/#/pull_requests/2022/1)

I've also always been interested in how torrents work, since I've used them before (for completely LEGAL purposes, of course). So naturally I thought, why not try to write a torrent client in Go? After all, from what I can tell, Go is usually used for backend, internet related things. I might be wrong though.

Of course I didn't make this alone, I'm not that smart. I followed [https://blog.jse.li/posts/torrent/](this post).

### Terminology

The **client** is us, the user. The **peer** is who we will be downloading the fragmented file from. A peer is identified by nothing more than an IP and a port address.

The **tracker** is a web service usually ending in the `/announce` endpoint, also in the announce field of the `.torrent` file. You visit it first, and it tells you a list of the **peers**. A **peer** is an IP/port combination that is available over the internet, that talks to the tracker periodically, and is also going to give you certain chunks of the file you desire. 

`.torrent` files are **bencoded**, which is a special kind of encoding. It's not too complex, and there's a nice Go library for this that we use to encode/decode.

A **piece** is simply a fragment of the entire file that we want to torrent. A **block** is even smaller than a piece, put blocks together to form pieces, put pieces together to form the file. Blocks are typically 16KB, and pieces are typically 256KB (so 16 pieces a block). Actually, the piece length *is* specified in the `.torrent` file. These sizes are changeable but there are limits on how big or small they can be and typically you should stick with the defaults.

![image](https://user-images.githubusercontent.com/69275171/181816646-2864bee0-6910-457c-b223-d83c618ff540.png)

The philosophy behind torrent is that instead of getting the entire file from one source, we split the file up into many pieces and then we connect to multiple sources, each of which gives us a piece of the whole file. Then we stitch together the pieces and get the whole file. This is arguably better because it reduces bandwith compared to the first approach, since I don't need to spend minutes, maybe even hours if its a big file, connected to the same server, which may be trying to service many other requests at the same time. Also, torrenting *may* arguably be faster since I can open up many connections at once. Torrenting is kind of like multithreading for downloads, if you think about it.

There are also two key ideas in BitTorrent, **choke** and **interested**. Basically choke means, if you send me a message, will I process it? And interested means, if I unchoke you, will I begin to request blocks from you? So basically:

`A block is downloaded by the client when the client is interested in a peer, and that peer is not choking the client. A block is uploaded by a client when the client is not choking a peer, and that peer is interested in the client.`

It's a little confusing but if still confused check out the spec. The most important fact is that the client starts out choking and uninterested in the peer, so before we request any blocks from the peer (after the handshake but before the first block request), we need to send an **unchoke** and **interested** message to the peer.

### The Process

The code works as follows:

1. Decode the bencoded `.torrent` file to figure out the address of the tracker
2. Create the right URL which will visit the tracker announce page, making sure to have the right query parameters like peerID or infoHash. We essentially have to ask the tracker, "what peers are available to download this file?"
3. Make the request to the announce page, get a bencoded response back
4. Decode the response again, and get the list of peers.
5. Connect to each peer (in code, this means starting a goroutine for each peer) and do a handshake with each one (start x goroutines where x is the number of peers)
6. Grab the **bitfield** from the peer's response to the handshake. The bitfield tells us which pieces of the file it owns
7. Create a []byte representing the handshake format, send it
8. Peer should send the exact same thing back, if not then sever the connection
9. Send a unchoke (ID 1) and interested (ID 2) message to the peer, because by default we ignore the peer's messages to us
10. Send a request message (ID 6) to the peer, asking for a block of this specific piece. We must specify piece index, starting position, and how many bytes we want of this piece (which is the blocksize)
11. Wait to receive a piece message (ID 7) back from the peer, which contains the requested block.
12. Once all blocks of a piece have been received, stitch them back together. Calculate the SHA1 hash and verify it with the value in the .torrent file for this piece
12. If the hashes match, send a have message (ID 4) to the peer for this specific piece index. This is to let the peer know that we (the client) have successfully downloaded and verified this piece's hash.
13. Do this for all pieces. Stitch pieces together to get final file. Profit!

### Useful References

- The [bittorrent specification](https://wiki.theory.org/BitTorrentSpecification) is really useful
- Here's [another](http://dandylife.net/docs/BitTorrent-Protocol.pdf) useful reference

### Code Layout
- `client` creates all the connections and sends all the requests (TCP and HTTP). Uses the "net" and "io" libraries
- `p2p` and `torrentfile` are the guts of the application that synchronizes all the pieces being downloaded, starts goroutines, etc.

In terms of abstraction- `main` calls `DownloadToFile` (torrentfile.go) which calls `Download` (p2p.go) which starts a bunch of goroutines (one for each peer) of type `startPeer` (p2p.go), which calls `tryDownloadPiece` (p2p.go) which calls `SendRequest` (client.go) repeatedly. That's the method stack trace. Pretty layered but it was relatively important that we kept things well separated so it doesn't get confusing.

### Running

Format is `.\gotorrent.exe [.torrent file] [path where you want finished file to be put]`

For example you can do `.\gotorrent.exe debian-11.4.0-amd64-netinst.iso.torrent debian.iso`

If not on windows you can do `go build` in project root and it should output a suitable executable for your OS

https://user-images.githubusercontent.com/69275171/181820674-340528cf-da3d-4c19-a38a-1f0e0d3b7f33.mp4

### Results
For one, when the client runs you can see the packets using Wireshark, which is super cool
![image](https://user-images.githubusercontent.com/69275171/181816349-f8b59929-4259-497b-bd6a-e28c19c8cd8f.png)

### Todos
- Look into magnet files
- Support UDP for announce?

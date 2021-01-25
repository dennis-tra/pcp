# `pcp` - Peer Copy
[![standard-readme compliant](https://img.shields.io/badge/readme%20style-standard-brightgreen.svg)](https://github.com/RichardLitt/standard-readme)
[![Go Report Card](https://goreportcard.com/badge/github.com/dennis-tra/pcp)](https://goreportcard.com/report/github.com/dennis-tra/pcp)
[![Maintainability](https://api.codeclimate.com/v1/badges/de64b09a3731b8a8842b/maintainability)](https://codeclimate.com/github/dennis-tra/pcp/maintainability)
[![Test Coverage](https://api.codeclimate.com/v1/badges/de64b09a3731b8a8842b/test_coverage)](https://codeclimate.com/github/dennis-tra/pcp/test_coverage)

Command line peer-to-peer data transfer tool based on [libp2p](https://github.com/libp2p/go-libp2p). 

## Table of Contents

- [Install](#install)
- [Usage](#usage)
- [Maintainers](#maintainers)
- [Contributing](#contributing)
- [License](#license)

## Install


## Usage

The receiving peer runs:

```shell
$ pcp receive
Your identity:

	16Uiu2HAkwyP8guhAXN66rkn8BEw3SjBavUuEu4VizTHhRcu7WLxq

Waiting for peers to connect... (cancel with strg+c)
```

The sending peer runs:

```shell
$ pcp send my_file
Querying peers that are waiting to receive files...

Found the following peer(s):
[0] 16Uiu2HAkwyP8guhAXN66rkn8BEw3SjBavUuEu4VizTHhRcu7WLxq

Select the peer you want to send the file to [#,r,q,?]:
```

At this point the sender needs to select the receiving peer, who in turn needs to confirm the file transfer.

## Development

### Generate Protobuf definitions

First install the protoc compiler:

```shell
go install google.golang.org/protobuf/cmd/protoc-gen-go
```

Then run from the root of this repository:

```shell
protoc -I=pkg/pb --go_out=pkg/pb --go_opt=paths=source_relative p2p.proto
```

The proto defintions were generated with `libprotoc 3.14.0`.

## Related Efforts

- [`croc`](https://github.com/schollz/croc) - Easily and securely send things from one computer to another
- [`dcp`](https://github.com/tom-james-watson/dat-cp) - Remote file copy, powered by the Dat protocol.
- [`iwant`](https://github.com/nirvik/iWant) - CLI based decentralized peer to peer file sharing
- [`p2pcopy`](https://github.com/psantosl/p2pcopy) - Small command line application to do p2p file copy behind firewalls without a central server.
- [`zget`](https://github.com/nils-werner/zget) - Filename based peer to peer file transfer
- [`sharedrop`](https://github.com/cowbell/sharedrop) - Easy P2P file transfer powered by WebRTC - inspired by Apple AirDrop
- [`filepizza`](https://github.com/kern/filepizza) - Peer-to-peer file transfers in your browser
- [`toss`](https://github.com/zerotier/toss) - Dead simple LAN file transfers from the command line

## Maintainers

[@dennis-tra](https://github.com/dennis-tra).

## Contributing

Feel free to dive in! [Open an issue](https://github.com/dennis-tra/pcp/issues/new) or submit PRs.

## License

[Apache License Version 2.0](LICENSE) © Dennis Trautwein

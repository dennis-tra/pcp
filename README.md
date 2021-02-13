# `pcp` - Peer Copy

[![standard-readme compliant](https://img.shields.io/badge/readme%20style-standard-brightgreen.svg)](https://github.com/RichardLitt/standard-readme)
[![Go Report Card](https://goreportcard.com/badge/github.com/dennis-tra/pcp)](https://goreportcard.com/report/github.com/dennis-tra/pcp)
[![Maintainability](https://api.codeclimate.com/v1/badges/de64b09a3731b8a8842b/maintainability)](https://codeclimate.com/github/dennis-tra/pcp/maintainability)
[![Test Coverage](https://api.codeclimate.com/v1/badges/de64b09a3731b8a8842b/test_coverage)](https://codeclimate.com/github/dennis-tra/pcp/test_coverage)

Command line peer-to-peer data transfer tool based on [libp2p](https://github.com/libp2p/go-libp2p).

![Demo animation](./docs/demo-2021-01-27.gif)

## Table of Contents

- [Table of Contents](#table-of-contents)
- [Project Status & Motivation](#project-status--motivation)
- [Install](#install)
- [Usage](#usage)
- [Development](#development)
  - [Generate Protobuf definitions](#generate-protobuf-definitions)
- [Feature Roadmap](#feature-roadmap)
- [Related Efforts](#related-efforts)
- [Maintainers](#maintainers)
- [Contributing](#contributing)
- [License](#license)

## Motivation

There already exists a long list of file transfer tools (see [Related Efforts](#related-efforts)), so why bother building another one?
The problem I had with the existing tools is that they rely on a [limited set of](https://github.com/schollz/croc/issues/289) [servers](https://magic-wormhole.readthedocs.io/en/latest/welcome.html#relays) to orchestrate peer matching and data relaying which poses a centralisation concern.
Many of the usual centralisation vs. decentralisation arguments apply here, e.g. the servers are single points of failures, the service operator has the power over whom to serve and whom not, etc. Further, as this [recent issue in croc](https://github.com/schollz/croc/issues/289) shows, this is a real risk for sustainable operation of the provided service.
 Only because a benevolent big player jumps in as a sponsor the service continues to exist.

## Description

`pcp` leverages the peer-to-peer networking stack of [libp2p](https://github.com/libp2p/go-libp2p).
It uses multicast DNS to find peers locally and the distributed hash table of IPFS for remote
peer discovery. The `identify` discovery mechanism serves the same role as `STUN`, but without the
need for a set of `STUN` servers. The libp2p `Circuit Relay` protocol allows peers to
communicate indirectly via a helpful intermediary peer that is found via the DHT. This replaces
dedicated `TURN` servers.

Of course there are some significant drawbacks with this approach: It's slower than established centralised methods if you want to transmit data over network boundaries. A DHT query to find your peer can take 2 - 3 minutes.

## Usage

The sending peer runs:

```shell
$ pcp send my_file
Code is:  bubble-enemy-result-increase
On the other machine run:
	pcp receive bubble-enemy-result-increase
```

The receiving peer runs:

```shell
$ pcp receive december-iron-old-increase
Looking for peer december-iron-old-increase...
```

## Install

### Release download

Head over to the [releases](https://github.com/dennis-tra/pcp/releases/tag/v0.1.1) and download the latest binary for your platform.

### Build from source

For now, you need to compile it yourself:

```shell
git clone https://github.com/dennis-tra/pcp.git
```

Navigate into the `pcp` folder and run:

```shell
go install cmd/pcp/pcp.go
```

Make sure the `$GOPATH/bin` is in your `PATH` variable to access the installed `pcp` executable.

### Package managers

It's on the roadmap to distribute `pcp` via `apt`, `yum`, `brew`, `scoop` etc

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

## Feature Roadmap

Shamelessly copied from `croc`:

- [x] Two computers on the same network can exchange files
- [ ] Two computers on the same network can exchange directories
- [ ] allows any two computers to transfer data (using a relay)
- [ ] provides end-to-end encryption (using PAKE)
- [ ] enables easy cross-platform transfers (Windows, Linux, Mac)
- [ ] allows multiple file transfers
- [ ] allows resuming transfers that are interrupted
- [ ] local server or port-forwarding not needed
- [ ] ipv6-first with ipv4 fallback
- [ ] can use proxy, like tor

## Related Efforts

- [`croc`](https://github.com/schollz/croc) - Easily and securely send things from one computer to another
- [`dcp`](https://github.com/tom-james-watson/dat-cp) - Remote file copy, powered by the Dat protocol.
- [`iwant`](https://github.com/nirvik/iWant) - CLI based decentralized peer to peer file sharing
- [`p2pcopy`](https://github.com/psantosl/p2pcopy) - Small command line application to do p2p file copy behind firewalls
  without a central server.
- [`zget`](https://github.com/nils-werner/zget) - Filename based peer to peer file transfer
- [`sharedrop`](https://github.com/cowbell/sharedrop) - Easy P2P file transfer powered by WebRTC - inspired by Apple
  AirDrop
- [`filepizza`](https://github.com/kern/filepizza) - Peer-to-peer file transfers in your browser
- [`toss`](https://github.com/zerotier/toss) - Dead simple LAN file transfers from the command line

## Maintainers

[@dennis-tra](https://github.com/dennis-tra).

## Acknowledgment

- [`progress`](https://github.com/machinebox/progress) - package to print progress

## Contributing

Feel free to dive in! [Open an issue](https://github.com/dennis-tra/pcp/issues/new) or submit PRs.

## License

[Apache License Version 2.0](LICENSE) © Dennis Trautwein

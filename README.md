# `pcp` - Peer Copy

[![standard-readme compliant](https://img.shields.io/badge/readme%20style-standard-brightgreen.svg)](https://github.com/RichardLitt/standard-readme)
[![Go Report Card](https://goreportcard.com/badge/github.com/dennis-tra/pcp)](https://goreportcard.com/report/github.com/dennis-tra/pcp)
[![Maintainability](https://api.codeclimate.com/v1/badges/de64b09a3731b8a8842b/maintainability)](https://codeclimate.com/github/dennis-tra/pcp/maintainability)
[![Test Coverage](https://api.codeclimate.com/v1/badges/de64b09a3731b8a8842b/test_coverage)](https://codeclimate.com/github/dennis-tra/pcp/test_coverage)

Command line peer-to-peer data transfer tool based on [libp2p](https://github.com/libp2p/go-libp2p).

![Demo animation](./docs/demo-2021-02-13.gif)

## Table of Contents

- [Motivation](#motivation)
- [Project Status](#project-status)
- [Usage](#usage)
- [Install](#install)
  - [Release download](#release-download) | [From source](#from-source) | [Package managers](#package-managers)
- [Development](#development)
  - [Protobuf definitions](#generate-protobuf-definitions)
- [Feature Roadmap](#feature-roadmap)
- [Related Efforts](#related-efforts)
- [Maintainers](#maintainers)
- [Acknowledgment](#acknowledgment)
- [Contributing](#contributing)
- [License](#license)

## Motivation

There already exists a long list of file transfer tools (see [Related Efforts](#related-efforts)), so why bother
building another one? The problem I had with the existing tools is that they rely on
a [limited set of](https://github.com/schollz/croc/issues/289) [servers](https://magic-wormhole.readthedocs.io/en/latest/welcome.html#relays)
to orchestrate peer matching and data relaying which poses a centralisation concern. Many of the usual centralisation
vs. decentralisation arguments apply here, e.g. the servers are single points of failures, the service operator has the
power over whom to serve and whom not, etc. Further, as
this [recent issue in croc](https://github.com/schollz/croc/issues/289) shows, this is a real risk for sustainable
operation of the provided service. Only because a benevolent big player jumps in as a sponsor the service continues to
exist.

`pcp` leverages the peer-to-peer networking stack of [libp2p](https://github.com/libp2p/go-libp2p). It uses multicast
DNS to find peers locally, and the distributed hash table of IPFS for remote peer discovery.

[comment]: <> (The `identify` discovery mechanism serves the same role as `STUN`, but without the need for a set of `STUN` servers. The libp2p `Circuit Relay` protocol allows peers to communicate indirectly via a helpful intermediary peer that is found via the DHT. This replaces dedicated `TURN` servers.)

## Project Status

The tool is in a very early stage, and I'm aware of performance and usability issues. Don't use it for anything serious. There is already a very good Go implementation named [`croc`](https://github.com/schollz/croc).

There are also drawbacks of this approach: It's slower than established centralised methods if
you want to transmit data across network boundaries. A DHT query to find your peer can take 2 - 3 minutes.
Further, the bandwidth and geographic location of a potential relaying peer is not guaranteed which can lead to long transmission times.

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

If you're on different networks the lookup can take quite long without any user facing output yet (~ 2-3 minutes).

## Install

### Release download

Head over to the [releases](https://github.com/dennis-tra/pcp/releases) and download the latest binary for
your platform.

### From source

For now, you need to compile it yourself:

```shell
git clone https://github.com/dennis-tra/pcp.git
```

Navigate into the `pcp` folder and run (at least `Go 1.13` required):

```shell
go install cmd/pcp/pcp.go
```

Make sure the `$GOPATH/bin` is in your `PATH` variable to access the installed `pcp` executable.

### Package managers

It's on the roadmap to distribute `pcp` via `apt`, `yum`, `brew`, `scoop` and more ...

## Development

### Protobuf definitions

First install the protoc compiler:

```shell
make tools # downloads gofumpt and protoc
make proto # generates protobuf
```

The current proto definitions were generated with `libprotoc 3.14.0`.

## Feature Roadmap

Shamelessly copied from `croc`:

- [x] allows any two computers to transfer data (using a relay)
  - ✅ using mDNS and DHT for peer discovery and [AutoRelay](https://docs.libp2p.io/concepts/circuit-relay/#autorelay) / [AutoNat](https://docs.libp2p.io/concepts/nat/#autonat)
- [x] provides end-to-end encryption (using PAKE)
  - ✅ actually PAKE is only used for authentication TLS for end-to-end encryption
- [x] enables easy cross-platform transfers (Windows, Linux, Mac)
  - 🤔✅ only tested Linux <-> Mac
- [ ] allows multiple file transfers
  - ❌ not yet
- [ ] allows resuming transfers that are interrupted
  - ❌ not yet
- [x] local server or port-forwarding not needed
  - ✅ thanks to [AutoNat](https://docs.libp2p.io/concepts/nat/#autonat)
- [ ] ipv6-first with ipv4 fallback
  - 🤔 I think that's the case, but I'm not sure about the libp2p internals
- [ ] can use proxy, like tor
  - ❌ not yet
  
## Related Efforts

- [`croc`](https://github.com/schollz/croc) - Easily and securely send things from one computer to another
- [`magic-wormhole`](https://github.com/magic-wormhole/magic-wormhole) - get things from one computer to another, safely
- [`dcp`](https://github.com/tom-james-watson/dat-cp) - Remote file copy, powered by the Dat protocol.
- [`iwant`](https://github.com/nirvik/iWant) - CLI based decentralized peer to peer file sharing
- [`p2pcopy`](https://github.com/psantosl/p2pcopy) - Small command line application to do p2p file copy behind firewalls
  without a central server.
- [`zget`](https://github.com/nils-werner/zget) - Filename based peer to peer file transfer
- [`sharedrop`](https://github.com/cowbell/sharedrop) - Easy P2P file transfer powered by WebRTC - inspired by Apple
  AirDrop
- [`filepizza`](https://github.com/kern/filepizza) - Peer-to-peer file transfers in your browser
- [`toss`](https://github.com/zerotier/toss) - Dead simple LAN file transfers from the command line
- Forgot yours? [Open an issue](https://github.com/dennis-tra/pcp/issues/new) or submit a PR :)

## Maintainers

[@dennis-tra](https://github.com/dennis-tra).

## Acknowledgment

- [`go-libp2p`](https://github.com/libp2p/go-libp2p) - The Go implementation of the libp2p Networking Stack.
- [`pake/v2`](https://github.com/schollz/pake/tree/v2.0.6) - PAKE library for generating a strong secret between parties over an insecure channel
- [`progressbar`](https://github.com/schollz/progressbar) - A really basic thread-safe progress bar for Golang applications

## Contributing

Feel free to dive in! [Open an issue](https://github.com/dennis-tra/pcp/issues/new) or submit PRs.

## License

[Apache License Version 2.0](LICENSE) © Dennis Trautwein

# `pcp` - Peer Copy

[![standard-readme compliant](https://img.shields.io/badge/readme%20style-standard-brightgreen.svg)](https://github.com/RichardLitt/standard-readme)
[![Go Report Card](https://goreportcard.com/badge/github.com/dennis-tra/pcp)](https://goreportcard.com/report/github.com/dennis-tra/pcp)
[![Maintainability](https://api.codeclimate.com/v1/badges/de64b09a3731b8a8842b/maintainability)](https://codeclimate.com/github/dennis-tra/pcp/maintainability)
[![Latest test suite run result](https://github.com/dennis-tra/pcp/actions/workflows/tests.yml/badge.svg)](https://github.com/dennis-tra/pcp/actions)
[![Github Releases Download Count](https://img.shields.io/github/downloads/dennis-tra/pcp/total.svg)]()

[comment]: <> ([![Test Coverage]&#40;https://api.codeclimate.com/v1/badges/de64b09a3731b8a8842b/test_coverage&#41;]&#40;https://codeclimate.com/github/dennis-tra/pcp/test_coverage&#41;)

Command line peer-to-peer data transfer tool based on [libp2p](https://github.com/libp2p/go-libp2p).

![Demo animation](./docs/demo-2023-05-01.gif)

_This tool was published at the [IFIP 2021](https://networking.ifip.org/2021/) conference. You can find the preprint [below](#research)._

## Table of Contents

- [Motivation](#motivation)
- [Project Status](#project-status)
- [How does it work?](#how-does-it-work)
- [Usage](#usage)
- [Install](#install)
  - [Release download](#release-download) | [From source](#from-source) | [Package managers](#package-managers)
- [Development](#development)
  - [Protobuf definitions](#generate-protobuf-definitions)
- [Feature Roadmap](#feature-roadmap)
- [Related Efforts](#related-efforts)
- [Maintainers](#maintainers)
- [Acknowledgments](#acknowledgments)
- [Research](#research)
- [License](#license)

## Motivation

There already exists a long list of file transfer tools (see [Related Efforts](#related-efforts)), so why bother
building another one? The problem I had with the existing tools is that they rely on
a [limited set of](https://github.com/schollz/croc/issues/289) [servers](https://magic-wormhole.readthedocs.io/en/latest/welcome.html#relays)
to orchestrate peer discovery and data relaying which poses a centralization concern. Many of the usual centralization
vs. decentralization arguments apply here, e.g. the servers are single points of failures, the service operator has the
power over whom to serve and whom not, etc. Further, as
this [issue in croc](https://github.com/schollz/croc/issues/289) shows, this is a real risk for sustainable
operation of the provided service.

[comment]: <> (The `identify` discovery mechanism serves the same role as `STUN`, but without the need for a set of `STUN` servers. The libp2p `Circuit Relay` protocol allows peers to communicate indirectly via a helpful intermediary peer that is found via the DHT. This replaces dedicated `TURN` servers.)

## Project Status

The tool has rough edges and there might be performance, usability and **security** issues. Don't use it for anything serious.
Although I criticised tools like [`magic-wormhole`](https://github.com/magic-wormhole/magic-wormhole) or [`croc`](https://github.com/schollz/croc) above, they are amazing and way more mature. I also borrow a lot from `croc`, e.g., [progressbar](https://github.com/schollz/progressbar) and `pake`(https://github.com/schollz/pake).

There are also drawbacks with this approach: It's slower than established centralised methods if
you want to transmit data over the internet. The tool needs to figure out information about the network you are currently in, and write a record in the public [IPFS DHT](https://docs.ipfs.tech/concepts/dht/). Then, after the receiving peer has found the network addresses of the sending peer it might need to perform a hole punch - further adding a few seconds of latency.

## How does it work?

When running `pcp send` you'll see four random words from the english words list of the Bitcoin improvement proposal [BIP39](https://github.com/bitcoin/bips/blob/master/bip-0039/bip-0039-wordlists.md).
The first word is interpreted as a _channel ID_ in the range from 0 to 2047 and the remaining ones are used for authentication and data transfer encryption in a later step.

`pcp` generates the _discovery identifier_ of the form `/pcp/{unix-nanos}/{channel-id}`, e.g., `/pcp/1682961900000000000/1886` and immediately starts broadcasting that string in your local network.
The unix nanos timestamp is the current time truncated to 5 minutes and the prefix `/pcp` is the protocol prefix.
In parallel, `pcp` analyzes your local network and tries to figure out your reachability (public or private) and, if your reachability is private, the type of [NAT](https://en.wikipedia.org/wiki/Network_address_translation) that you're behind (cone or symmetric).
If `pcp` finds that you are in a private network behind a cone NAT it [makes reservations](https://github.com/libp2p/specs/blob/master/relay/circuit-v2.md) at random peers in the IPFS public DHT that will later help [punching a hole](https://blog.ipfs.tech/2022-01-20-libp2p-hole-punching/) through that NAT.
This is crucial for direct connectivity.

After `pcp` knows more about your network, it crafts a [CID](https://docs.ipfs.tech/concepts/content-addressing/) from the discovery identifier and writes a [provider-record](https://docs.ipfs.tech/concepts/dht/) in the IPFS public DHT.
This provider record points from the CID eventually to your network addresses (public, private and relay reservations).
After this step is done you can pass the four random words to your friend.


[//]: # (There are lists in nine different languages of 2048 words each, currently only `english` is supported.)

To receive the file the your friend must enter `pcp receive four-words-from-above`. `pcp` then uses the first word together with the current time truncated to 5 minutes to generate the same discovery identifier.
It then goes ahead and looks for that in your local network via mDNS and the public IPFS DHT by crafting the identical CID as above.
It also searches for an identifier of the previous 5-minute interval.
If the receiving peer finds the sending peer in your local network, both start a password authenticated key exchange ([PAKE](https://en.wikipedia.org/wiki/Password-authenticated_key_agreement)) to authenticate each other.
If the receiving peer finds the provider record of the sending peer in the DHT it tries to connect to the network addresses that are present in the provider-record.
If a direct connection is not possible, `pcp` uses the relay reservation to perform a [direct connection upgrade through relay](https://github.com/libp2p/specs/blob/master/relay/DCUtR.md) and punch a hole through either NAT.

After a direct connection was successfully established, both peers perform a password authenticated key exchange ([PAKE](https://en.wikipedia.org/wiki/Password-authenticated_key_agreement)) to authenticate each other.
In this procedure a comparably weak password (`four-words-from-above`) gets replaced with a strong session key that is used to encrypt all future communication.
The default [TLS](https://en.wikipedia.org/wiki/Transport_Layer_Security) encryption that [libp2p](https://github.com/libp2p/go-libp2p) provides is not sufficient in this case as we could still, in theory, talk to a wrong peer - just encrypted.
More information about this in [Brian Warner's awesome talk at PyCon 2016](https://www.youtube.com/watch?v=oFrTqQw0_3c) about [Magic Wormhole](https://github.com/magic-wormhole/magic-wormhole).

After the peer is authenticated the receiver must confirm the file transfer, and the file gets transmitted.

## Usage

The sending peer runs:

```shell
$ pcp send my_file
On the other machine run:
	pcp receive bubble-enemy-result-increase
```

The receiving peer runs:

```shell
$ pcp receive bubble-enemy-result-increase
Looking for peer bubble-enemy-result-increase...
```

If you're on different networks the preparation on the sending side can take ~1 minute.

## Install

### Package managers

```shell
brew install pcp
```

It's on the roadmap to also distribute `pcp` via `apt`, `yum`, `scoop` and more ...

### Release download

Head over to the [releases](https://github.com/dennis-tra/pcp/releases) and download the latest archive for
your platform.

### From source

To compile it yourself run:

```shell
go install github.com/dennis-tra/pcp/cmd/pcp@latest # Go 1.20 or higher is required
```

Make sure the `$GOPATH/bin` is in your `PATH` variable to access the installed `pcp` executable.

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
  - ✅ yup, it uses [`pake/v2`](https://github.com/schollz/pake/tree/v2.0.6) from `croc`
- [x] enables easy cross-platform transfers (Windows, Linux, Mac)
  - ✅ Linux <-> Mac, ❌ Windows, but it's planned!
- [x] allows multiple file transfers
  - ✅ it allows transferring directories
- [ ] allows resuming transfers that are interrupted
  - ❌ not yet
- [x] local server or port-forwarding not needed
  - ✅ thanks to [AutoNat](https://docs.libp2p.io/concepts/nat/#autonat)
- [x] ipv6-first with ipv4 fallback
  - ✅ thanks to [libp2p](https://libp2p.io/)
- [ ] can use proxy, like tor
  - ❌ not yet

You can find a project plan in the [project tab](https://github.com/dennis-tra/pcp/projects/2) of this page.
Some other ideas I would love to work on include:

- [x] experimental decentralised NAT hole punching via DHT signaling servers - [Project Flare](https://github.com/libp2p/go-libp2p/issues/1039)
- [ ] browser interop via the means of [js-libp2p](https://github.com/libp2p/js-libp2p)

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

## Acknowledgments

- [`go-libp2p`](https://github.com/libp2p/go-libp2p) - The Go implementation of the libp2p Networking Stack.
- [`pake/v2`](https://github.com/schollz/pake/tree/v2.0.6) - PAKE library for generating a strong secret between parties over an insecure channel
- [`progressbar`](https://github.com/schollz/progressbar) - A really basic thread-safe progress bar for Golang applications

## Contributing

Feel free to dive in! [Open an issue](https://github.com/dennis-tra/pcp/issues/new) or submit PRs.

## Research

This tool was submitted to the [International Federation for Information Processing 2021 (IFIP '21)](https://networking.ifip.org/2021/) conference and accepted for publication. You can find the preprint [here](docs/trautwein2021.pdf).

<details>
 <summary>Cite the paper with this BibTeX entry:</summary>
  <img src="docs/jesse-pinkman.jpg" alt="Jesse Pinkman" width="300">
</details>

```bibtex
@inproceedings{Trautwein2021,
  title        = {Introducing Peer Copy - A Fully Decentralized Peer-to-Peer File Transfer Tool},
  author       = {Trautwein, Dennis and Schubotz, Moritz and Gipp, Bela},
  year         = 2021,
  month        = {June},
  booktitle    = {2021 IFIP Networking Conference (IFIP Networking)},
  publisher    = {IEEE},
  address      = {Espoo and Helsinki, Finland},
  doi          = {10.23919/IFIPNetworking52078.2021.9472842},
  note         = {ISBN 978-3-9031-7639-3},
  topic        = {misc}
}
```

## Support

It would really make my day if you supported this project through [Buy Me A Coffee](https://www.buymeacoffee.com/dennistra).

## Other Projects

You may be interested in one of my other projects:

- [`image-stego`](https://github.com/dennis-tra/image-stego) - A novel way to image manipulation detection. Steganography-based image integrity - Merkle tree nodes embedded into image chunks so that each chunk's integrity can be verified on its own.
- [`nebula-crawler`](https://github.com/dennis-tra/nebula-crawler) - A libp2p DHT crawler that also monitors the liveness and availability of peers. 🏆 Winner of the [DI2F Workshop Hackathon](https://research.protocol.ai/blog/2021/decentralising-the-internet-with-ipfs-and-filecoin-di2f-a-report-from-the-trenches) 🏆

## License

[Apache License Version 2.0](LICENSE) © Dennis Trautwein

# pcp
[![standard-readme compliant](https://img.shields.io/badge/readme%20style-standard-brightgreen.svg?style=flat-square)](https://github.com/RichardLitt/standard-readme) [![Go Report Card](https://goreportcard.com/badge/github.com/dennis-tra/pcp)](https://goreportcard.com/report/github.com/dennis-tra/pcp)

`p`eer `c`o`p`y - Command line peer-to-peer data transfer tool. 

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

- [Name](link) - description

## Maintainers

[@dennis-tra](https://github.com/dennis-tra).

## Contributing

Feel free to dive in! [Open an issue](https://github.com/dennis-tra/pcp/issues/new) or submit PRs.

## License

[Apache License Version 2.0](LICENSE) © Dennis Trautwein

# pcp
[![standard-readme compliant](https://img.shields.io/badge/readme%20style-standard-brightgreen.svg?style=flat-square)](https://github.com/RichardLitt/standard-readme)

Command line peer-to-peer data distribution tool. Peer CoPy

## Table of Contents

- [Install](#install)
- [Usage](#usage)
- [Maintainers](#maintainers)
- [Contributing](#contributing)
- [License](#license)

## Install


## Usage

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

# swaggrpc

[![GoDoc][doc-img]][doc] [![Build Status][ci-img]][ci]

[gRPC](https://grpc.io/) wrapper around [swagger](https://swagger.io) (Open API) services.

This uses [openapi2proto](https://github.com/NYTimes/openapi2proto) to generate a
[protocol buffer](https://developers.google.com/protocol-buffers/) service definition from a swagger
specification, then uses [protoreflect](https://github.com/jhump/protoreflect) to serve the new API.
Swagger calls are made using [go-openapi](https://github.com/go-openapi).

## Building

This project uses [dep](https://github.com/golang/dep) to manage dependencies.

To build, first run:

```
dep ensure -vendor-only
```

Then:

```
make all
```

[doc-img]: https://godoc.org/github.com/Nordstrom/swaggrpc?status.svg
[doc]: https://godoc.org/github.com/Nordstrom/swaggrpc
[ci-img]: https://travis-ci.org/Nordstrom/swaggrpc.svg
[ci]: https://travis-ci.org/Nordstrom/swaggrpc

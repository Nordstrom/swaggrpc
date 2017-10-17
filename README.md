# swaggrpc
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

# REST-NATS

REST-NATS is a simple server that accepts HTTP requests on a port, and publishes
an equivalent message on NATS.

The URL for the request is converted to a subject by removing the leading separator on the URL,
and replacing all separators with a `.`, i.e. `http://server:port/hello/world` converts to `hello.world`.
The payload if any becomes the message body. Any requests to `/` are illegal, since the leading
separator is removed and would result in an empty subject.

REST-NATS supports an embedded mode, in which it runs an embedded NATS server.


### Options

```
  -e    Embed gnatsd (for testing)
  -hp string
        NATS host port (default "localhost:4222")
  -w string
        HTTP host port (default "localhost:8080")
```


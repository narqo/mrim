# mrim

A Go client for Mail.Ru Agent aka Mail.Ru Instant Messenger (mrim).

[![GoDoc](https://godoc.org/github.com/narqo/mrim?status.svg)](http://godoc.org/github.com/narqo/mrim)
[![BCH compliance](https://bettercodehub.com/edge/badge/narqo/mrim?branch=master)](https://bettercodehub.com/)

---

**The project is NOT meant to be finished. Yet. Contributions are welcome.**

---

## Basic Usage

```go
// Creare a client new instance.
opts := &mrim.Options{
    Addr:       "mrim.mail.ru:2042",
    ClientDesc: "go mrim client 1.0",
    Username:   "example@mail.ru",
    Password:   "****",
}
c, err := mrim.NewClient(ctx, opts)
if err != nil {
    ...
}
defer c.Close()

// Send packet to the server.
p := mrim.Packet{
    Header: mrim.Header{
        Msg: mrim.MsgCSPing,
    }
}
err = c.Send(ctx, p)

// Read incoming packets.
for {
    p, err := c.Recv()

    switch p.Header.Msg {
    case mrim.MsgCSMessageAck:
        // got unread message
    }
}
```

## See Also

- https://github.com/mailru/mrasender

---

## License

WTFPL

# smtpproxy, a Simple Mail Proxy

[![Build Status](https://api.travis-ci.org/jorgenschaefer/smtpproxy.png?branch=master)](https://travis-ci.org/jorgenschaefer/smtpproxy)

## Overview

`smtpproxy` is a small Go program that accepts messages on port 25 and
relays them directly to a relay host, without local storage. This is
useful for forwarding your own e-mail addresses to gmail or similar
services. `smtpproxy` also does some minimal spam detection.

## Installation

`smtpproxy` is best run from `systemd`. It uses environment variables
for configuration. See [`example/defaults`](example/defaults) for the
list of supported options.

```
go get github.com/jorgenschaefer/smtpproxy
cp $GOPATH/bin/smtpproxy /usr/local/sbin/
cp $GOPATH/src/github.com/jorgenschaefer/smtpproxy/example/smtpproxy.service \
   /etc/systemd/system/smtpproxy.service
cp $GOPATH/src/github.com/jorgenschaefer/smtpproxy/example/defaults \
   /etc/default/smtpproxy
systemctl daemon-reload
$EDITOR /etc/default/smtpproxy
systemctl start smtpproxy.service
```

## Features

- No local spool or storage at all. The client only receives a success
  message when the upstream server accepts the mail.
- Minimum implementation as per
  [RFC 5321](https://www.ietf.org/rfc/rfc5321.txt) section 4.5.1, with
  the exception of `VRFY`.
- The `STARTTLS` extension is supported.
- Delayed welcome: The 220 welcome message is sent with a short delay.
  If the client speaks before its turn, it is tarpitted. This catches
  a surprising amount of spammers.
- Tarpit: When a client misbehaves in a bad way, the connection is
  kept open for some time to slow down spammers.

## Contributing

Contributions are welcome. Please do make sure tests run successfully.
There’s a `./scripts/test` command to run the test suite. When adding
code, please try to include tests as well.

This is my first Go program. (Helpful) suggestions for improving the
code are very welcome.

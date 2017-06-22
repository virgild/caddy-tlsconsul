# caddy-tlsconsul

[Consul](https://github.com/hashicorp/consul) Storage for [Caddy](https://github.com/mholt/caddy) TLS data. 

Normally, Caddy uses local filesystem to store TLS data when it auto-generates certificates from a CA like Lets Encrypt.
This plugin enables Caddy to store TLS data like user key and certificates in Consul's KV store. This allows you to use Caddy in a 
cluster or multi machine environment with a centralized storage for auto-generated certificates. 

With this plugin it is possible to use multiple Caddy instances with the same HTTPS domain for instance with DNS round-robin.

It works with recent versions of Caddy 0.10.x
All saved data gets encrypted using AES.

## Installation

You need to compile Caddy by yourself to use this plugin. Alternativly you can use my Docker image that already includes Consul KV storage, more infos below.

- Set up a working Go installation, see https://golang.org/doc/install
- Checkout Caddy source code from https://github.com/mholt/caddy
- Get latest caddy-tlsconsul with `go get -u github.com/pteich/caddy-tlsconsul`
- Add this line to `caddy/caddymain/run.go` in the `import` region:
```go
import (
  ...
  _ "github.com/pteich/caddy-tlsconsul"
)
```
- Change dir into `caddy/caddymain` and compile Caddy with `build.bash`

## Configuration

In order to use Consul you have to change the storage provider in your Caddyfile like so:

```
    tls my@email.com {
        storage consul
    }
```

Because this plugin uses the official Consul API client you can use all ENV variables like `CONSUL_HTTP_ADDR` or `CONSUL_HTTP_TOKEN`
to define your Consul connection and credentials. For more information see https://github.com/hashicorp/consul/blob/master/api/api.go

Without any further configuration a running Consul on 127.0.01:8500 is assumed.

There are additional ENV variables for this plugin:

- `CADDY_CONSULTLS_AESKEY` defines your personal AES key to use when encrypting data. It needs to be 32 characters long.
- `CADDY_CONSULTLS_PREFIX` defines the prefix for the keys in KV store. Default is `caddytls`

## Run with Docker

You can use a custom version of Caddy with integrated Consul TLS storage using the Dockerfile provided in this repo. Because this Dockerfile uses multi-stage build you need at least Docker 17.05 CE.

See https://hub.docker.com/r/pteich/caddy-tlsconsul/

```bash
docker run -d -p 80:80 -p 443:443 -v /home/test/Caddyfile:/Caddyfile:ro -v /home/test/config:/.caddy:rw -v /home/test/html:/var/www/html pteich/caddy-tlsconsul -agree -conf=/Caddyfile
```

# Caddy Storage Consul K/V

[Consul K/V](https://github.com/hashicorp/consul) Storage for [Caddy](https://github.com/mholt/caddy) TLS data. 

By default Caddy uses local filesystem to store TLS data (generated keys, csr, crt) when it auto-generates certificates from a CA like Lets Encrypt.
Starting with 0.11.x Caddy can work in cluster environments where TLS storage path is shared across servers. 
This is a great improvement but you need to take care of mounting a centeralized storage on every server. If you have an already running Consul cluster it can be easier to use it's KV store to save certificates and make them available to all Caddy instances.

This plugin enables Caddy to store TLS data like keys and certificates in Consul's K/V store. 
This allows you to use Caddy in a cluster or multi machine environment and use a centralized storage for auto-generated certificates. 

With this plugin it is possible to use multiple Caddy instances with the same HTTPS domain for instance with DNS round-robin.
All data that is saved in KV store is encrypted using AES.

The version of this plugin in master branch is supposed to work with versions of Caddy that use https://github.com/mholt/certmagic and
its new storage interface (> 0.11.1). More at https://github.com/pteich/caddy-tlsconsul/issues/3 

For older versions of Caddy (0.10.x - 0.11.1) you can use the old_storage_interface branch.

## Installation (subject to change for Caddy >0.11.1)

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
- [DEPRECATED] ~~Change dir into `caddy/caddymain` and compile Caddy with `build.bash`~~
- Change dir into `caddy/caddy` do a `go get github.com/caddyserver/builds` and compile Caddy with `go run build.go`

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

Example for a Docker run command:
```bash
docker run -d -p 80:80 -p 443:443 -e "CONSUL_HTTP_ADDR=my.consul.addr:8500" -v /home/test/Caddyfile:/Caddyfile:ro -v /home/test/config:/.caddy:rw -v /home/test/html:/var/www/html pteich/caddy-tlsconsul -agree -conf=/Caddyfile
```

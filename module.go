package storageconsul

import (
	"strconv"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/certmagic"
)

func init() {
	caddy.RegisterModule(Storage{})
}

func (Storage) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID: "caddy.storage.consul",
		New: func() caddy.Module {
			return New()
		},
	}
}

// Provision is called by Caddy to prepare the module
func (s *Storage) Provision(ctx caddy.Context) error {
	s.logger = ctx.Logger(s).Sugar()
	s.logger.Infof("TLS storage is using Consul at %s", s.config.ConsulAddr)
	return s.createConsulClient()
}

func (s *Storage) CertMagicStorage() (certmagic.Storage, error) {
	return s, nil
}

// UnmarshalCaddyfile parses plugin settings from Caddyfile
// storage consul {
//     address      "127.0.0.1:8500"
//     token        "consul-access-token"
//     timeout      10
//     prefix       "caddytls"
//     value_prefix "myprefix"
//     aes_key      "consultls-1234567890-caddytls-32"
//     tls_enabled  "false"
//     tls_insecure "true"
// }
func (s *Storage) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		args := d.RemainingArgs()
		if len(args) > 1 {
			switch args[0] {
			case "address":
				if args[1] != "" {
					parsedAddress, err := caddy.ParseNetworkAddress(args[1])
					if err == nil {
						WithConsulAddr(parsedAddress.JoinHostPort(0))(&s.config)
					}
				}
			case "token":
				if args[1] != "" {
					WithConsulToken(args[1])(&s.config)
				}
			case "timeout":
				if args[1] != "" {
					timeParse, err := strconv.Atoi(args[1])
					if err == nil {
						WithTimeout(time.Duration(timeParse) * time.Second)(&s.config)
					}
				}
			case "prefix":
				if args[1] != "" {
					WithPrefix(args[1])(&s.config)
				}
			case "value_prefix":
				if args[1] != "" {
					WithValuePrefix(args[1])(&s.config)
				}
			case "aes_key":
				if args[1] != "" {
					WithAESKey(args[1])
				}
			case "tls_enabled":
				if args[1] != "" {
					tlsParse, err := strconv.ParseBool(args[1])
					if err == nil {
						WithConsulTls(tlsParse)(&s.config)
					}
				}
			case "tls_insecure":
				if args[1] != "" {
					tlsInsecureParse, err := strconv.ParseBool(args[1])
					if err == nil {
						WithConsulTlsInsecure(tlsInsecureParse)
					}
				}
			}
		} else {
			continue
		}
	}
	return nil
}

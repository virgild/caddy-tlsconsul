package storageconsul

import (
	"os"
	"strconv"

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
	s.logger.Infof("TLS storage is using Consul at %s", s.Address)

	// override default values from ENV
	if aesKey := os.Getenv(EnvNameAESKey); aesKey != "" {
		s.AESKey = []byte(aesKey)
	}

	if prefix := os.Getenv(EnvNamePrefix); prefix != "" {
		s.Prefix = prefix
	}

	if valueprefix := os.Getenv(EnvValuePrefix); valueprefix != "" {
		s.ValuePrefix = valueprefix
	}

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
		key := d.Val()
		var value string

		if !d.Args(&value) {
			continue
		}

		switch key {
		case "address":
			if value != "" {
				parsedAddress, err := caddy.ParseNetworkAddress(value)
				if err == nil {
					s.Address = parsedAddress.JoinHostPort(0)
				}
			}
		case "token":
			if value != "" {
				s.Token = value
			}
		case "timeout":
			if value != "" {
				timeParse, err := strconv.Atoi(value)
				if err == nil {
					s.Timeout = timeParse
				}
			}
		case "prefix":
			if value != "" {
				s.Prefix = value
			}
		case "value_prefix":
			if value != "" {
				s.ValuePrefix = value
			}
		case "aes_key":
			if value != "" {
				s.AESKey = []byte(value)
			}
		case "tls_enabled":
			if value != "" {
				tlsParse, err := strconv.ParseBool(value)
				if err == nil {
					s.TlsEnabled = tlsParse
				}
			}
		case "tls_insecure":
			if value != "" {
				tlsInsecureParse, err := strconv.ParseBool(value)
				if err == nil {
					s.TlsInsecure = tlsInsecureParse
				}
			}
		}
	}
	return nil
}

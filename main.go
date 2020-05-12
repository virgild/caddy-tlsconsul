package storageconsul

import (
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/certmagic"
)

type TLSConsul struct {
	storage *ConsulStorage
}

func init() {
	caddy.RegisterModule(TLSConsul{})
}

func (TLSConsul) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "caddy.storage.tlsconsul",
		New: func() caddy.Module { return new(TLSConsul) },
	}
}

// Provision is called by Caddy to prepare the module
func (tlsc *TLSConsul) Provision(ctx caddy.Context) error {
	consulStorage, err := NewConsulStorage()
	if err != nil {
		return err
	}
	tlsc.storage = consulStorage
	return nil
}

func (tlsc TLSConsul) CertMagicStorage() (certmagic.Storage, error) {
	return tlsc.storage, nil
}

package tlsconsul

import (
	"fmt"
	"net/url"
	"path"

	"os"

	"github.com/hashicorp/consul/api"
	"github.com/mholt/caddy/caddytls"
)

const (
	DefaultPrefix = "caddytls"

	// AES Key needs to be either 16 or 32 bytes
	DefaultAESKey = "consultls-1234567890-caddytls-32"

	EnvNameAESKey = "CADDY_CONSULTLS_AESKEY"
	EnvNamePrefix = "CADDY_CONSULTLS_PREFIX"
)

func init() {
	caddytls.RegisterStorageProvider("consul", NewConsulStorage)
}

func NewConsulStorage(caURL *url.URL) (caddytls.Storage, error) {

	consulCfg := api.DefaultConfig()
	consulClient, err := api.NewClient(consulCfg)
	if err != nil {
		return nil, fmt.Errorf("Unable to create Consul client: %v", err)
	}
	if _, err := consulClient.Agent().NodeName(); err != nil {
		return nil, fmt.Errorf("Unable to ping Consul: %v", err)
	}

	cs := &ConsulStorage{
		consulClient: consulClient,
		caHost:       caURL.Host,
		prefix:       DefaultPrefix,
		aesKey:       []byte(DefaultAESKey),
		locks:        make(map[string]*api.Lock),
	}

	if aesKey := os.Getenv(EnvNameAESKey); aesKey != "" {
		cs.aesKey = []byte(aesKey)
	}

	if prefix := os.Getenv(EnvNamePrefix); prefix != "" {
		cs.prefix = prefix
	}

	return cs, nil
}

type ConsulStorage struct {
	consulClient    *api.Client
	caHost          string
	prefix          string
	lockTTLSeconds  int
	lockWaitSeconds int
	aesKey          []byte
	locks           map[string]*api.Lock
}

func (cs *ConsulStorage) key(suffix string) string {
	return path.Join(cs.prefix, cs.caHost, suffix)
}

func (cs *ConsulStorage) eventKey() string {
	return cs.key("domainevent")
}

func (cs *ConsulStorage) siteKey(domain string) string {
	return cs.key(path.Join("sites", domain))
}

func (cs *ConsulStorage) userKey(email string) string {
	return cs.key(path.Join("users", email))
}

func (cs *ConsulStorage) SiteExists(domain string) (bool, error) {
	kv, _, err := cs.consulClient.KV().Get(cs.siteKey(domain), &api.QueryOptions{RequireConsistent: true})
	if err != nil {
		return false, err
	}
	return kv != nil, nil
}

func (cs *ConsulStorage) LoadSite(domain string) (*caddytls.SiteData, error) {
	var err caddytls.ErrNotExist
	kv, _, err := cs.consulClient.KV().Get(cs.siteKey(domain), &api.QueryOptions{RequireConsistent: true})
	if err != nil {
		return nil, fmt.Errorf("Unable to obtain site data for %v: %v", domain, err)
	} else if kv == nil {
		return nil, err
	}
	ret := new(caddytls.SiteData)
	if err = cs.fromBytes(kv.Value, ret); err != nil {
		return nil, fmt.Errorf("Unable to decode site data for %v: %v", domain, err)
	}
	return ret, nil
}

func (cs *ConsulStorage) StoreSite(domain string, data *caddytls.SiteData) error {
	kv := &api.KVPair{Key: cs.siteKey(domain)}
	var err error
	kv.Value, err = cs.toBytes(data)
	if err != nil {
		return fmt.Errorf("Unable to encode site data for %v: %v", domain, err)
	}
	if _, err = cs.consulClient.KV().Put(kv, nil); err != nil {
		return fmt.Errorf("Unable to store site data for %v: %v", domain, err)
	}
	// We need to fire an event here to invalidate the cache elsewhere
	evt := &api.UserEvent{Name: cs.eventKey()}
	if evt.Payload, err = cs.toBytes(domain); err != nil {
		return fmt.Errorf("Unable to create domain-changed event for %v: %v", domain, err)
	}
	// TODO: we know that we are going to receive our own event. Should I store the
	// resulting ID somewhere so I know not to act on it and reload it? Or is it
	// harmless to reload it?
	if _, _, err = cs.consulClient.Event().Fire(evt, nil); err != nil {
		return fmt.Errorf("Unable to send domain-changed event for %v: %v", domain, err)
	}
	return nil
}

func (cs *ConsulStorage) DeleteSite(domain string) error {

	var err caddytls.ErrNotExist

	// In order to delete properly and know whether it took, we must first
	// get and do a CAS operation because delete is idempotent
	// (ref: https://github.com/hashicorp/consul/issues/348). This can
	// cause race conditions on multiple servers. But since this is a
	// user-initiated action (i.e. revoke), they will see the error.
	kv, _, err := cs.consulClient.KV().Get(cs.siteKey(domain), &api.QueryOptions{RequireConsistent: true})
	if err != nil {
		return fmt.Errorf("Unable to obtain site data for %v: %v", domain, err)
	} else if kv == nil {
		return err
	}
	if success, _, err := cs.consulClient.KV().DeleteCAS(kv, nil); err != nil {
		return fmt.Errorf("Unable to delete site data for %v: %v", domain, err)
	} else if !success {
		return fmt.Errorf("Failed to lock site data delete for %v", domain)
	}
	// TODO: on revoke, what do we do here? Send out an event?
	return nil
}

func (cs *ConsulStorage) lockKey(domain string) string {
	return cs.key(path.Join("locks", domain))
}

func (cs *ConsulStorage) TryLock(domain string) (caddytls.Waiter, error) {
	// We can trust this isn't double called in the same process
	opts := &api.LockOptions{
		Key:         cs.lockKey(domain),
		LockTryOnce: true,
	}
	lock, err := cs.consulClient.LockOpts(opts)
	if err != nil {
		return nil, fmt.Errorf("Failed creating lock for %v: %v", domain, err)
	}
	leaderCh, err := lock.Lock(nil)
	if err != nil && err != api.ErrLockHeld {
		return nil, fmt.Errorf("Unexpected error attempting to take lock for %v: %v", domain, err)
	} else if leaderCh == nil || err != nil {
		return nil, fmt.Errorf("No leader channel when attempting to take lock for %v", domain)
	}
	// We don't care if we lose the leaderCh...
	cs.locks[domain] = lock
	return nil, nil
}

func (cs *ConsulStorage) Unlock(domain string) error {
	if lock := cs.locks[domain]; lock != nil {
		if err := lock.Unlock(); err != nil && err != api.ErrLockNotHeld {
			return fmt.Errorf("Failed unlocking lock for %v: %v", domain, err)
		}
	}
	return nil
}

func (cs *ConsulStorage) LoadUser(email string) (*caddytls.UserData, error) {
	kv, _, err := cs.consulClient.KV().Get(cs.userKey(email), &api.QueryOptions{RequireConsistent: true})
	if err != nil {
		return nil, caddytls.ErrNotExist(fmt.Errorf("Unable to obtain user data for %v: %v", email, err))
	} else if kv == nil {
		return nil, caddytls.ErrNotExist(fmt.Errorf("not found"))
	}

	user := new(caddytls.UserData)
	if err = cs.fromBytes(kv.Value, user); err != nil {
		return nil, fmt.Errorf("Unable to decode site data for %v: %v", email, err)
	}
	return user, nil
}

func (cs *ConsulStorage) StoreUser(email string, data *caddytls.UserData) error {
	kv := &api.KVPair{Key: cs.userKey(email)}

	var err error
	if kv.Value, err = cs.toBytes(data); err != nil {
		return fmt.Errorf("Unable to encode user data for %v: %v", email, err)
	}
	if _, err = cs.consulClient.KV().Put(kv, nil); err != nil {
		return fmt.Errorf("Unable to store user data for %v: %v", email, err)
	}

	return nil
}

func (cs *ConsulStorage) MostRecentUserEmail() string {
	panic("no impl - MostRecentUserEmail")
}

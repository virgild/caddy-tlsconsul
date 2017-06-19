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
	// DefaultPrefix defines the default prefix in KV store
	DefaultPrefix = "caddytls"

	// DefaultAESKey needs to be 32 bytes long
	DefaultAESKey = "consultls-1234567890-caddytls-32"

	// EnvNameAESKey defines the env variable name to override AES key
	EnvNameAESKey = "CADDY_CONSULTLS_AESKEY"

	// EnvNamePrefix defines the env variable name to override KV key prefix
	EnvNamePrefix = "CADDY_CONSULTLS_PREFIX"
)

func init() {
	caddytls.RegisterStorageProvider("consul", NewConsulStorage)
}

// NewConsulStorage connects to Consul and returns a caddytls.Storage for the specific caURL
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

// ConsulStorage holds all parameters for the Consul connection
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

func (cs *ConsulStorage) siteKey(domain string) string {
	return cs.key(path.Join("sites", domain))
}

func (cs *ConsulStorage) userKey(email string) string {
	return cs.key(path.Join("users", email))
}

// SiteExists checks if a cert for a specific domain already exists
func (cs *ConsulStorage) SiteExists(domain string) (bool, error) {
	kv, _, err := cs.consulClient.KV().Get(cs.siteKey(domain), &api.QueryOptions{RequireConsistent: true})
	if err != nil {
		return false, err
	}
	return kv != nil, nil
}

// LoadSite loads the site data for a domain from Consul KV
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

// StoreSite stores the site data for a given domain in Consul KV
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

	return nil
}

// DeleteSite deletes site data for a given domain
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

	return nil
}

func (cs *ConsulStorage) lockKey(domain string) string {
	return cs.key(path.Join("locks", domain))
}

// TryLock sets a lock for a given domain in KV
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

// Unlock releases an existing lock
func (cs *ConsulStorage) Unlock(domain string) error {
	if lock := cs.locks[domain]; lock != nil {
		if err := lock.Unlock(); err != nil && err != api.ErrLockNotHeld {
			return fmt.Errorf("Failed unlocking lock for %v: %v", domain, err)
		}
	}
	return nil
}

// LoadUser loads user data for a given email address
func (cs *ConsulStorage) LoadUser(email string) (*caddytls.UserData, error) {
	kv, _, err := cs.consulClient.KV().Get(cs.userKey(email), &api.QueryOptions{RequireConsistent: true})
	if err != nil {
		return nil, caddytls.ErrNotExist(fmt.Errorf("Unable to obtain user data for %v: %v", email, err))
	} else if kv == nil {
		return nil, caddytls.ErrNotExist(fmt.Errorf("not found"))
	}

	user := new(caddytls.UserData)
	if err = cs.fromBytes(kv.Value, user); err != nil {
		return nil, fmt.Errorf("Unable to decode user data for %v: %v", email, err)
	}
	return user, nil
}

// StoreUser stores user data for a given email address in KV store
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

// MostRecentUserEmail returns the last modified email address from KV store
func (cs *ConsulStorage) MostRecentUserEmail() string {
	kvpairs, _, err := cs.consulClient.KV().List(cs.key("users"), &api.QueryOptions{RequireConsistent: true})
	if err != nil {
		return ""
	}

	if len(kvpairs) == 0 {
		return ""
	}

	userIndex := 0
	var lastModified uint64

	for i, kv := range kvpairs {
		if kv.ModifyIndex > lastModified {
			userIndex = i
		}
		lastModified = kv.ModifyIndex
	}

	_, email := path.Split(kvpairs[userIndex].Key)

	return email
}

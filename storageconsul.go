package storageconsul

import (
	"fmt"
	"net"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	consul "github.com/hashicorp/consul/api"
	"github.com/mholt/caddy"
	"github.com/mholt/certmagic"
)

const (
	// DefaultPrefix defines the default prefix in KV store
	DefaultPrefix = "caddytls"

	// DefaultAESKey needs to be 32 bytes long
	DefaultAESKey = "consultls-1234567890-caddytls-32"

	// DefaultValuePrefix sets a prefix to KV values to check validation
	DefaultValuePrefix = "caddy-storage-consul"

	// EnvNameAESKey defines the env variable name to override AES key
	EnvNameAESKey = "CADDY_CLUSTERING_CONSUL_AESKEY"

	// EnvNamePrefix defines the env variable name to override KV key prefix
	EnvNamePrefix = "CADDY_CLUSTERING_CONSUL_PREFIX"

	// EnvValuePrefix defines the env variable name to override KV value prefix
	EnvValuePrefix = "CADDY_CLUSTERING_CONSUL_VALUEPREFIX"
)

// dialContext to use for Consul connection
var dialContext = (&net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 15 * time.Second,
}).DialContext

// StorageData describes the data that is saved to KV
type StorageData struct {
	Value    []byte
	Modified time.Time
}

// ConsulStorage holds all parameters for the Consul connection
type ConsulStorage struct {
	ConsulClient *consul.Client
	prefix       string
	valuePrefix  string
	aesKey       []byte
	locks        map[string]*consul.Lock
}

// Implementation of certmagic.Waiter
type consulStorageWaiter struct {
	key          string
	waitDuration time.Duration
	wg           *sync.WaitGroup
}

func (csw *consulStorageWaiter) Wait() {
	csw.wg.Add(1)
	go time.AfterFunc(csw.waitDuration, func() {
		csw.wg.Done()
	})
	csw.wg.Wait()
}

var constructConsulClusterPlugin caddy.ClusterPluginConstructor

func init() {
	caddy.RegisterClusterPlugin("file", constructConsulClusterPlugin)
}

// NewConsulStorage connects to Consul and returns a ConsulStorage
func NewConsulStorage() (*ConsulStorage, error) {

	// get the default config
	consulCfg := consul.DefaultConfig()
	// set our special dialcontext to prevent default keepalive
	consulCfg.Transport.DialContext = dialContext

	// create the Consul API client
	consulClient, err := consul.NewClient(consulCfg)
	if err != nil {
		return nil, fmt.Errorf("Unable to create Consul client: %v", err)
	}
	if _, err := consulClient.Agent().NodeName(); err != nil {
		return nil, fmt.Errorf("Unable to ping Consul: %v", err)
	}

	// create ConsulStorage and pre-set values
	cs := &ConsulStorage{
		ConsulClient: consulClient,
		prefix:       DefaultPrefix,
		aesKey:       []byte(DefaultAESKey),
		valuePrefix:  DefaultValuePrefix,
		locks:        make(map[string]*consul.Lock),
	}

	// override default values from ENV
	if aesKey := os.Getenv(EnvNameAESKey); aesKey != "" {
		cs.aesKey = []byte(aesKey)
	}

	if prefix := os.Getenv(EnvNamePrefix); prefix != "" {
		cs.prefix = prefix
	}

	if valueprefix := os.Getenv(EnvValuePrefix); valueprefix != "" {
		cs.valuePrefix = valueprefix
	}

	return cs, nil
}

// helper function to prefix key
func (cs *ConsulStorage) prefixKey(key string) string {
	return path.Join(cs.prefix, key)
}

// TryLock aquires a lock for the given key or returns a waiter
func (cs ConsulStorage) TryLock(key string) (certmagic.Waiter, error) {

	// if we already hold the lock, return early
	if _, exists := cs.locks[key]; exists {
		return nil, nil
	}

	lock, err := cs.ConsulClient.LockOpts(&consul.LockOptions{
		Key:          key,
		LockTryOnce:  true,
		LockWaitTime: 1 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	leaderCh, err := lock.Lock(nil)
	if err != nil {
		return nil, err
	}

	// key is already locked by another client, return a waiter
	if leaderCh == nil {
		waiter := &consulStorageWaiter{
			key:          key,
			waitDuration: consul.DefaultLockRetryTime,
			wg:           new(sync.WaitGroup),
		}
		return waiter, nil
	}

	cs.locks[key] = lock

	return nil, nil
}

// Unlock releases a specific lock
func (cs ConsulStorage) Unlock(key string) error {
	if lock, exists := cs.locks[key]; exists {
		err := lock.Unlock()
		if err != nil {
			delete(cs.locks, key)
		} else {
			return err
		}
	}
	return nil
}

// UnlockAllObtained releases all locks
func (cs ConsulStorage) UnlockAllObtained() {
	for key := range cs.locks {
		cs.Unlock(key)
	}
}

// Store saves encrypted value at key in Consul KV
func (cs ConsulStorage) Store(key string, value []byte) error {

	kv := &consul.KVPair{Key: cs.prefixKey(key)}

	consulData := StorageData{
		Value:    value,
		Modified: time.Now(),
	}

	encryptedValue, err := cs.toBytes(consulData)
	if err != nil {
		return fmt.Errorf("Unable to encode data for %v: %v", key, err)
	}

	kv.Value = encryptedValue

	if _, err = cs.ConsulClient.KV().Put(kv, nil); err != nil {
		return fmt.Errorf("Unable to store data for %v: %v", key, err)
	}

	return nil
}

// Load retrieves the value for key from Consul KV
func (cs ConsulStorage) Load(key string) ([]byte, error) {
	kv, _, err := cs.ConsulClient.KV().Get(cs.prefixKey(key), &consul.QueryOptions{RequireConsistent: true})
	if err != nil {
		return nil, fmt.Errorf("Unable to obtain data for %s: %v", key, err)
	} else if kv == nil {
		return nil, certmagic.ErrNotExist(fmt.Errorf("Key %s not exists", key))
	}

	contents := &StorageData{}
	err = cs.fromBytes(kv.Value, contents)
	if err != nil {
		return nil, fmt.Errorf("Unable to decrypt data for %s: %v", key, err)
	}

	return contents.Value, nil
}

// Delete a key
func (cs ConsulStorage) Delete(key string) error {

	// first obtain existing keypair
	kv, _, err := cs.ConsulClient.KV().Get(cs.prefixKey(key), &consul.QueryOptions{RequireConsistent: true})
	if err != nil {
		return fmt.Errorf("Unable to obtain data for %s: %v", key, err)
	} else if kv == nil {
		return certmagic.ErrNotExist(err)
	}

	// no do a Check-And-Set operation to verify we really deleted the key
	if success, _, err := cs.ConsulClient.KV().DeleteCAS(kv, nil); err != nil {
		return fmt.Errorf("Unable to delete data for %s: %v", key, err)
	} else if !success {
		return fmt.Errorf("Failed to lock data delete for %s", key)
	}

	return nil
}

// Exists checks if a key exists
func (cs ConsulStorage) Exists(key string) bool {
	kv, _, err := cs.ConsulClient.KV().Get(cs.prefixKey(key), &consul.QueryOptions{RequireConsistent: true})
	if kv != nil && err == nil {
		return true
	}
	return false
}

// List returns a list with all keys under a given prefix
func (cs ConsulStorage) List(prefix string, recursive bool) ([]string, error) {
	var keysFound []string

	// get a list of all keys at prefix
	keys, _, err := cs.ConsulClient.KV().Keys(cs.prefixKey(prefix), "", &consul.QueryOptions{RequireConsistent: true})
	if err != nil {
		return keysFound, err
	}

	if len(keys) == 0 {
		return keysFound, certmagic.ErrNotExist(fmt.Errorf("No keys at %s", prefix))
	}

	// remove default prefix from keys
	for _, key := range keys {
		if strings.HasPrefix(key, cs.prefixKey(prefix)) {
			key = strings.TrimPrefix(key, cs.prefix+"/")
			keysFound = append(keysFound, key)
		}
	}

	// if recursive wanted, just return all keys
	if recursive {
		return keysFound, nil
	}

	// for non-recursive split path and look for unique keys just under given prefix
	keysMap := make(map[string]bool)
	for _, key := range keysFound {
		dir := strings.Split(strings.TrimPrefix(key, prefix+"/"), "/")
		keysMap[dir[0]] = true
	}

	keysFound = make([]string, 0)
	for key := range keysMap {
		keysFound = append(keysFound, path.Join(prefix,key))
	}

	return keysFound, nil
}

// Stat returns statistic data of a key
func (cs ConsulStorage) Stat(key string) (certmagic.KeyInfo, error) {

	kv, _, err := cs.ConsulClient.KV().Get(cs.prefixKey(key), &consul.QueryOptions{RequireConsistent: true})
	if err != nil {
		return certmagic.KeyInfo{}, fmt.Errorf("Unable to obtain data for %s: %v", key, err)
	} else if kv == nil {
		return certmagic.KeyInfo{}, certmagic.ErrNotExist(fmt.Errorf("Key %s does not exist", key))
	}

	contents := &StorageData{}
	err = cs.fromBytes(kv.Value, contents)
	if err != nil {
		return certmagic.KeyInfo{}, fmt.Errorf("Unable to decrypt data for %s: %v", key, err)
	}

	return certmagic.KeyInfo{
		Key:        key,
		Modified:   contents.Modified,
		Size:       int64(len(contents.Value)),
		IsTerminal: false,
	}, nil
}

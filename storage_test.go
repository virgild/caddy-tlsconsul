package tlsconsul

import (
	"net/url"
	"testing"
	"os"

	"github.com/hashicorp/consul/api"
	"github.com/mholt/caddy/caddytls"
	"reflect"
)

var consulClient *api.Client

const TestPrefix = "consultlstest"
const TestCaUrl = "http://consultls.test"

// these tests need and running Consul, the address and other options can be set as usual via env variables
// see https://github.com/hashicorp/consul/blob/master/api/api.go
func setupConsulEnv(t *testing.T) caddytls.Storage {

	os.Setenv(EnvNamePrefix, TestPrefix)
	os.Setenv(api.HTTPTokenEnvName, "consul-4-sabc")

	var err error
	consulCfg := api.DefaultConfig()
	if consulClient, err = api.NewClient(consulCfg); err != nil {
		t.Fatalf("Error creating Consul client: %v", err)
	}
	if _, err := consulClient.Agent().NodeName(); err != nil {
		t.Fatalf("Unable to ping Consul: %v", err)
	}

	cleanupConsulEnv(t)

	caurl, _ := url.Parse(TestCaUrl)
	cs, err := NewConsulStorage(caurl)
	if err != nil {
		t.Fatalf("Error creating Consul storage: %v", err)
	}

	return cs
}

func cleanupConsulEnv(t *testing.T) {

	if _, err := consulClient.KV().DeleteTree(TestPrefix, nil); err != nil {
		t.Fatalf("error deleting KV tree %s: %v", TestPrefix, err)
	}
}

func getUser() *caddytls.UserData {
	return &caddytls.UserData{
		Reg: []byte("registration"),
		Key: []byte("key"),
	}
}

func TestMostRecentUserEmail(t *testing.T) {
	cs := setupConsulEnv(t)

	email := cs.MostRecentUserEmail()
	if email != "" {
		t.Fatalf("email should be empty if nothing found")
	}

	cs.StoreUser("test@test.com", getUser())
	email = cs.MostRecentUserEmail()
	if email != "test@test.com" {
		t.Fatalf("email not matching if one user exists")
	}

	cs.StoreUser("test2@test.com", getUser())
	email = cs.MostRecentUserEmail()
	if email != "test2@test.com" {
		t.Fatalf("email should be the newest user but found %s", email)
	}

	cleanupConsulEnv(t)
}

func TestStoreAndLoadUser(t *testing.T) {
	cs := setupConsulEnv(t)

	defaultUser := getUser()
	err := cs.StoreUser("test@test.com", defaultUser)
	if err != nil {
		t.Fatalf("Error storing user: %v", err)
	}

	user, err := cs.LoadUser("test@test.com")
	if err != nil {
		t.Fatalf("Error loading user: %v", err)
	}
	if !reflect.DeepEqual(user, defaultUser) {
		t.Fatalf("Loaded user is not the same like the saved one")
	}

	cleanupConsulEnv(t)
}

package main

import (
	"fmt"

	"github.com/hashicorp/consul/api"

	"github.com/pteich/caddy-tlsconsul"
)

func main() {
	cs := storageconsul.New()

	cs.Prefix = "caddy2-streamurls"
	cs.AESKey = []byte("sabc4ever-1234567890-caddytls-32")

	consulConfig := api.DefaultConfig()
	consulConfig.Address = "consul.streamabc.link:8500"
	consulConfig.Token = "78a023fa-113d-5f86-602e-1485cacf1cc3"
	client, err := api.NewClient(consulConfig)
	if err != nil {
		panic(err)
	}
	cs.ConsulClient = client

	c, err := cs.Load("/certificates/acme-v02.api.letsencrypt.org-directory/caddytest.quantumcast.cloud/caddytest.quantumcast.cloud.crt")
	if err != nil {
		panic(err)
	}

	sd, err := cs.DecryptStorageData(c)
	if err != nil {
		panic(err)
	}

	fmt.Printf("########%s", string(sd.Value))
}

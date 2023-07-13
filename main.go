package main

import (
	"encoding/base64"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/hekmon/transmissionrpc"
	"github.com/mmcdole/gofeed"
	"gopkg.in/yaml.v3"
)

var config struct {
	Feeds  []string `yaml:"feeds"`
	Server struct {
		Host    string `yaml:"host"`
		Port    int    `yaml:"port"`
		TLS     bool   `yaml:"tls"`
		RPCPath string `yaml:"rpc_path"`
	} `yaml:"server"`
	Login struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"login"`
	UpdateInterval int `yaml:"update_interval"`
}

type Torrent struct {
	Title string
	Link  string
}

func main() {
	yamlFile, err := ioutil.ReadFile("/etc/transmission-rss.conf")
	if err != nil {
		log.Fatalf("Open config file: %v", err)
	}

	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}

	for {
		doWork()
		log.Printf("Sleep %d seconds...", config.UpdateInterval)
		time.Sleep(time.Duration(config.UpdateInterval) * time.Second)
	}
}

func doWork() {
	torrents := make([]Torrent, 0)

	parser := gofeed.NewParser()
	for _, feedURL := range config.Feeds {
		feed, err := parser.ParseURL(feedURL)
		if err != nil {
			log.Printf("Parse feed: %v", err)
			continue
		}
		for _, item := range feed.Items {
			if len(item.Enclosures) == 0 || item.Enclosures[0].URL == "" {
				log.Printf("No link for %s", item.Title)
				continue
			}

			link, err := url.Parse(item.Enclosures[0].URL)
			if err != nil {
				log.Printf("Fail to parse URL: %v", err)
				continue
			}
			torrent := Torrent{
				Title: item.Title,
				Link:  link.String(),
			}
			torrents = append(torrents, torrent)
		}
	}

	//TODO: filter torrents already added

	if config.Server.RPCPath == "" {
		config.Server.RPCPath = "/transmission/rpc"
	}

	transmissionbt, err := transmissionrpc.New(config.Server.Host, config.Login.Username, config.Login.Password, &transmissionrpc.AdvancedConfig{
		RPCURI: config.Server.RPCPath,
		HTTPS:  config.Server.TLS,
		Port:   uint16(config.Server.Port),
	})
	if err != nil {
		log.Fatalf("Create transmission client: %v", err)
	}

	client := http.Client{
		Timeout: 10 * time.Second,
	}

	for _, torrent := range torrents {
		resp, err := client.Get(torrent.Link)
		if err != nil {
			log.Printf("download torrent: %v", err)
			continue
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("fail to get body: %v", body)
			continue
		}

		base64Torrent := base64.RawStdEncoding.EncodeToString(body)

		_, err = transmissionbt.TorrentAdd(&transmissionrpc.TorrentAddPayload{
			MetaInfo: &base64Torrent,
		})
		if err != nil {
			log.Printf("add torrent: %v", err)
		}

		// TODO: improve logging
		log.Printf("Torrent added: %s", torrent.Title)
	}
}

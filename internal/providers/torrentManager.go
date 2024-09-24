package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

type TorrentManager struct {
	logger *zerolog.Logger
}

type ProviderConfig struct {
	Name          string `json:"name"`
	BaseUrl       string `json:"url"`
	SearchUrl     string `json:"searchUrl"`
	Enabled       bool   `json:"enabled"`
	Type          string `json:"type"`
	Debug         bool   `json:"debug"`
	ItemSelector  string `json:"itemSelector"`
	ItemsSelector struct {
		DetailUrl         string `json:"detail_url"`
		Title             string `json:"title"`
		Seeds             string `json:"seeds"`
		Peers             string `json:"peers"`
		Size              string `json:"size"`
		MagnetPreffixLink string `json:"magnetPreffixLink"`
		MagnetSelector    string `json:"magnetSelector"`
	} `json:"itemsSelector"`
	Trackers []string `json:"trackers,omitempty"`
}

func NewTorrentManager(logger *zerolog.Logger) *TorrentManager {
	return &TorrentManager{
		logger: logger,
	}
}

func (p *TorrentManager) readConfigFile(file string) (*ProviderConfig, error) {
	var config ProviderConfig
	configFile, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer configFile.Close()
	jsonParser := json.NewDecoder(configFile)
	jsonParser.Decode(&config)
	return &config, nil
}

func (p *TorrentManager) loadProviderConfig(provider string) (*ProviderConfig, error) {
	cfg, err := p.readConfigFile(fmt.Sprintf("internal/providers/config/%s.json", provider))
	if err != nil {
		p.logger.Err(err).Msgf("error while getting provider %s config file: %v", provider, err)
		return nil, err
	}
	return cfg, err
}

func (p *TorrentManager) loadAllProviderConfig() ([]*ProviderConfig, error) {
	var config []*ProviderConfig
	var files []string
	err := filepath.Walk("./internal/providers/config", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		configFile, err := p.readConfigFile(file)
		if err != nil {
			return nil, err
		}
		config = append(config, configFile)
	}
	return config, nil
}

func (p *TorrentManager) GetActiveProviders() ([]*ProviderConfig, error) {
	var config []*ProviderConfig
	cfg, err := p.loadAllProviderConfig()
	if err != nil {
		return nil, err
	}
	for _, conf := range cfg {
		if conf.Enabled {
			config = append(config, conf)
		}
	}
	return config, nil
}

func (p *TorrentManager) FetchAllActive(ctx context.Context, params SearchParams) ([]*TorrentItem, error) {
	var items []*TorrentItem
	cfg, err := p.GetActiveProviders()
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	for _, conf := range cfg {
		wg.Add(1)
		go func(ctx context.Context, conf *ProviderConfig, params SearchParams) {
			provider := NewTorrentProvider(conf, p.logger)
			torrents := provider.FetchAndParse(ctx, params)
			items = append(items, torrents...)
			wg.Done()
		}(ctx, conf, params)
	}
	wg.Wait()
	return p.postFilter(items, params), nil
}

func (p *TorrentManager) FetchByProvider(ctx context.Context, provider string, params SearchParams) ([]*TorrentItem, error) {

	cfg, err := p.loadProviderConfig(provider)
	if err != nil {
		return nil, err
	}
	torrentProvider := NewTorrentProvider(cfg, p.logger)

	torrents := torrentProvider.FetchAndParse(ctx, params)
	return p.postFilter(torrents, params), nil
}

func (p *TorrentManager) postFilter(items []*TorrentItem, params SearchParams) []*TorrentItem {
	var filtered []*TorrentItem
	p.logger.Info().Msgf("Total items received to be filtered: %d", len(items))
	for _, item := range items {

		if params.Filters.Group != "" && !strings.Contains(item.Group, params.Filters.Group) && !strings.Contains(strings.ToLower(item.OriginalTitle), params.Filters.Group) {
			fmt.Printf("Filtering didn't match: %s, %s, %s\n", params.Filters.Group, item.Group, item.OriginalTitle)
			continue
		}

		if params.Filters.Resolution == "" {
			filtered = append(filtered, item)
			continue
		}

		var torrents []Torrent
		for _, torrent := range item.Torrents {
			if strings.Contains(strings.ToLower(torrent.Resolution), strings.ToLower(params.Filters.Resolution)) {
				torrents = append(torrents, torrent)
			}
		}

		if len(torrents) > 0 {
			item.Torrents = torrents
			filtered = append(filtered, item)
		}
	}
	p.logger.Info().Msgf("Total filtered: %d", len(filtered))
	return filtered
}

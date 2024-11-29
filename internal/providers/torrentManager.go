package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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
	cfg, err := p.readConfigFile(fmt.Sprintf("./internal/providers/config/%s.json", provider))
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

func (p *TorrentManager) FetchAllActive(ctx context.Context, params SearchParams) ([]*Torrent, error) {
	var items []*Torrent
	cfg, err := p.GetActiveProviders()
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	for _, conf := range cfg {
		wg.Add(1)
		go func(ctx context.Context, conf *ProviderConfig, params SearchParams) {
			defer wg.Done()
			provider := NewTorrentProvider(conf, p.logger)
			torrents := provider.FetchAndParse(ctx, params)
			items = append(items, torrents...)
		}(ctx, conf, params)
	}
	wg.Wait()
	return p.postFilter(items, params), nil
}

func (p *TorrentManager) FetchByProvider(ctx context.Context, provider string, params SearchParams) ([]*Torrent, error) {

	cfg, err := p.loadProviderConfig(provider)
	if err != nil {
		return nil, err
	}
	torrentProvider := NewTorrentProvider(cfg, p.logger)

	torrents := torrentProvider.FetchAndParse(ctx, params)
	return p.postFilter(torrents, params), nil
}

func (p *TorrentManager) sizeToBytes(sizeStr string) (int64, error) {
	sizeStr = strings.TrimSpace(sizeStr)
	var size float64
	var unit string

	if _, err := fmt.Sscanf(sizeStr, "%f %s", &size, &unit); err != nil {
		return 0, err
	}

	switch strings.ToUpper(unit) {
	case "GB":
		return int64(size * 1024 * 1024 * 1024), nil // convert gb to bytez
	case "MB":
		return int64(size * 1024 * 1024), nil // convert mb to bytez
	case "KB":
		return int64(size * 1024), nil // convert kb to bytez
	case "B":
		return int64(size), nil
	default:
		return 0, fmt.Errorf("invalid unit %s", unit)
	}
}

func (p *TorrentManager) postFilter(items []*Torrent, params SearchParams) []*Torrent {
	var filtered []*Torrent
	p.logger.Info().Msgf("Total items received to be filtered: %d", len(items))
	minSize, _ := p.sizeToBytes("700 MB")
	maxSize, _ := p.sizeToBytes("3 GB")
	minSerieSize, _ := p.sizeToBytes("250 MB")
	maxSerieSize, _ := p.sizeToBytes("1.5 GB")

	// filtering by params and size (default)
	for _, item := range items {
		sizeInBytes, err := p.sizeToBytes(item.Size)
		if err != nil {
			p.logger.Info().Msgf("error while casting size: %s, item: %s", err.Error(), item.Title)
			continue
		}
		if params.Filters.Resolution != "" && !strings.Contains(strings.ToLower(item.Resolution), strings.ToLower(params.Filters.Resolution)) {
			continue
		}

		if params.Filters.Group != "" && !strings.Contains(item.Group, params.Filters.Group) && !strings.Contains(strings.ToLower(item.OriginalTitle), params.Filters.Group) {
			continue
		}

		if !strings.EqualFold(item.Title, params.Filters.Title) || strings.EqualFold(item.OriginalTitle, params.Filters.Title) {
			continue
		}

		if strings.EqualFold(item.Quality, "HDCAM") {
			continue
		}

		switch item.Type {
		case "movie":
			if sizeInBytes < minSize || sizeInBytes > maxSize {
				continue
			}

		case "serie":
			if sizeInBytes < minSerieSize || sizeInBytes > maxSerieSize {
				continue
			}
			if item.Season != params.Filters.Season && item.Episode != params.Filters.Episode {
				continue
			}
		}

		filtered = append(filtered, item)
	}

	// sort by size
	sort.Slice(filtered, func(i, j int) bool {
		sizeI, errI := p.sizeToBytes(filtered[i].Size)
		sizeJ, errJ := p.sizeToBytes(filtered[j].Size)
		if errI != nil || errJ != nil {
			return false
		}
		return sizeI < sizeJ
	})

	p.logger.Info().Msgf("Total filtered: %d", len(filtered))
	return filtered
}

package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/gocolly/colly/v2"
	"github.com/rs/zerolog"
	parsetorrentname "github.com/xochilpili/go-parse-torrent-name"
)

type TorrentProvider struct {
	c      *colly.Collector
	rs     *resty.Client
	config *ProviderConfig
	logger *zerolog.Logger
}

func NewTorrentProvider(config *ProviderConfig, logger *zerolog.Logger) *TorrentProvider {
	c := colly.NewCollector(
		colly.MaxDepth(2),
		colly.Async(true),
		colly.UserAgent("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"),
	)
	rs := resty.New()
	return &TorrentProvider{
		c:      c,
		rs:     rs,
		config: config,
		logger: logger,
	}
}

func (t *TorrentProvider) FetchAndParse(ctx context.Context, params SearchParams) []*TorrentItem {
	var result []*TorrentItem
	if t.config.Type == "html" {
		result = t.fetchByScrappe(ctx, params)
		return result
	}
	result = t.fetchByApi(ctx, params)
	return result
}

func (t *TorrentProvider) fetchByScrappe(ctx context.Context, params SearchParams) []*TorrentItem {
	_, cancel := context.WithCancel(ctx)
	defer cancel()

	t.c.Limit(&colly.LimitRule{Parallelism: 2, RandomDelay: 5 * time.Second})

	itemSet := make(map[string]bool)
	itemChan := make(chan *TorrentItem)
	var wg sync.WaitGroup
	var mu sync.Mutex

	var items []*TorrentItem

	t.c.OnHTML(t.config.ItemSelector, func(h *colly.HTMLElement) {
		detailUrl := h.ChildAttr(t.config.ItemsSelector.DetailUrl, "href")
		title := h.ChildText(t.config.ItemsSelector.Title)
		strSeeds := h.ChildText(t.config.ItemsSelector.Seeds)
		strPeers := h.ChildText(t.config.ItemsSelector.Peers)
		size := h.ChildText(t.config.ItemsSelector.Size)
		
		parsedTitle := strings.ReplaceAll(title, " ", "-")

		info, err := t.parseTorrentTitle(parsedTitle)
		if err != nil {
			//t.logger.Panic().Err(err).Msg("Title parser error")
			t.logger.Err(err).Msg("error parsing title")
		}

		itemType := "movie"
		if info.Episode != 0 {
			itemType = "serie"
		}

		seeds, err := strconv.Atoi(strSeeds)
		if err != nil {
			seeds = 0
		}

		peers, err := strconv.Atoi(strPeers)
		if err != nil {
			peers = 0
		}

		torrent := Torrent{
			Resolution: info.Resolution,
			Codec:      info.Codec,
			Quality:    info.Quality,
			Size:       size,
			Seeds:      seeds,
			Peers:      peers,
		}
		parsedTitle = strings.Trim(strings.ReplaceAll(info.Title, "-", " "), " ")
		re := regexp.MustCompile(`\(|\[`)
		parsedTitle = strings.TrimSpace(re.ReplaceAllString(parsedTitle, ""))
		item := &TorrentItem{
			Provider:      t.config.Name,
			Type:          itemType,
			Title:         parsedTitle,
			OriginalTitle: title,
			Year:          info.Year,
			Group:         strings.ToLower(info.Group),
			Season:        info.Season,
			Episode:       info.Episode,
			Torrents:      []Torrent{torrent},
		}

		if strings.Contains(detailUrl, t.config.ItemsSelector.MagnetPreffixLink) {
			baseUrl := fmt.Sprintf("%s%s", t.config.BaseUrl, detailUrl)
			wg.Add(1)
			go func(link string, item *TorrentItem, itemChan chan<- *TorrentItem, wg *sync.WaitGroup) {
				defer wg.Done()
				c := colly.NewCollector()
				c.OnHTML(t.config.ItemsSelector.MagnetSelector, func(h *colly.HTMLElement) {
					magnetStr := h.Attr("href")
					if magnetStr != "" {
						if !itemSet[item.OriginalTitle] {
							mu.Lock()
							itemSet[item.OriginalTitle] = true
							mu.Unlock()
							item.Torrents[0].Magnet = magnetStr
							itemChan <- item
						}
					}
				})
				c.Visit(link)
			}(baseUrl, item, itemChan, &wg)
		}
	})

	if t.config.Debug {
		t.c.OnResponse(func(r *colly.Response) {
			fmt.Printf("%s", string(r.Body))
		})
	}

	baseUrl := fmt.Sprintf("%s%s", t.config.BaseUrl, strings.Replace(t.config.SearchUrl, "{query}", params.Query, 1))
	t.logger.Info().Msgf("Scrapping: %s", baseUrl)

	t.c.Visit(baseUrl)
	t.c.Wait()

	go func() {
		wg.Wait()
		close(itemChan)
	}()

	for item := range itemChan {
		items = append(items, item)
	}

	t.logger.Info().Msgf("Proiver: %s, got %d results", t.config.Name, len(items))
	return items
}

func (t *TorrentProvider) fetchByApi(ctx context.Context, params SearchParams) []*TorrentItem {
	baseUrl := fmt.Sprintf("%s%s", t.config.BaseUrl, strings.Replace(t.config.SearchUrl, "{query}", params.Query, 1))
	t.logger.Info().Msgf("Fetch API: %s", baseUrl)

	resp, err := t.rs.R().SetHeader("Content-Type", "application/json").SetContext(ctx).Get(baseUrl)
	if err != nil {
		t.logger.Panic().Err(err).Msgf("error while fetching: %s, %v", baseUrl, err)
	}

	items, err := t.transform2Item(resp.Body())
	if err != nil {
		t.logger.Panic().Err(err).Msg("error while transform object types")
	}

	t.logger.Info().Msgf("Provider: %s, got %d results", t.config.Name, len(items))

	return items
}

func (t *TorrentProvider) transform2Item(data []byte) ([]*TorrentItem, error) {
	var tpbItems []TPBItem
	err := json.Unmarshal(data, &tpbItems)
	if err == nil {
		var items []*TorrentItem
		for _, el := range tpbItems {
			info, err := t.parseTorrentTitle(el.Name)
			if err != nil {
				return nil, err
			}

			if info.Title == "" {
				continue
			}

			itemType := "movie"

			if info.Season != 0 {
				itemType = "serie"
			}

			peers, err := strconv.Atoi(el.Peers)
			if err != nil {
				peers = 0
			}
			seeds, err := strconv.Atoi(el.Seeds)
			if err != nil {
				seeds = 0
			}

			parsedTitle := strings.Trim(strings.ReplaceAll(info.Title, "-", " "), " ")
			re := regexp.MustCompile(`\(|\[`)
			parsedTitle = strings.TrimSpace(re.ReplaceAllString(parsedTitle, ""))

			item := &TorrentItem{
				Provider:      t.config.Name,
				Title:         parsedTitle,
				OriginalTitle: el.Name,
				Type:          itemType,
				Year:          info.Year,
				Group:         strings.ToLower(info.Group),
				Episode:       info.Episode,
				Season:        info.Season,
				Torrents: []Torrent{{
					Resolution: info.Resolution,
					Quality:    info.Quality,
					Codec:      info.Codec,
					Seeds:      seeds,
					Peers:      peers,
					Size:       t.formatSize(el.Size),
					Magnet:     t.formatMagnet(el.InfoHash, el.Name)},
				}}
			items = append(items, item)
		}
		return items, nil
	}

	var ytsItems YtsPopularRootObject
	err = json.Unmarshal(data, &ytsItems)
	if err == nil {
		var items []*TorrentItem
		for _, ytsItem := range ytsItems.Data.Movies {
			var torrents []Torrent
			for _, ytsTorrent := range ytsItem.Torrents {
				torrent := Torrent{
					Resolution: ytsTorrent.Quality,
					Quality:    ytsTorrent.Type,
					Codec:      ytsTorrent.VideoCodec,
					Seeds:      ytsTorrent.Seeds,
					Peers:      ytsTorrent.Peers,
					Size:       ytsTorrent.Size,
					Magnet:     t.formatMagnet(ytsTorrent.Hash, ytsItem.Title),
				}
				torrents = append(torrents, torrent)
			}
			item := &TorrentItem{
				Provider:      t.config.Name,
				Type:          "movie", // YTS only has movies
				Title:         ytsItem.TitleEnglish,
				OriginalTitle: ytsItem.Title,
				Year:          ytsItem.Year,
				Group:         "yts",
				Torrents:      torrents,
			}
			items = append(items, item)
		}
		return items, nil
	}
	return nil, errors.New("unable to cast type")
}

func (t *TorrentProvider) formatMagnet(infoHash string, name string) string {
	var trackers []string
	for _, tracker := range t.config.Trackers {
		trackers = append(trackers, url.PathEscape(tracker))
	}
	trackerStr := fmt.Sprintf("&tr=%s", strings.Join(trackers, "&tr="))
	magnetStr := fmt.Sprintf("magnet:?xt=urn:btih:%s&dn=%s%s", infoHash, url.PathEscape(name), trackerStr)
	return magnetStr
}

func (t *TorrentProvider) formatSize(strSize string) string {
	const (
		MB = 1024 * 1024
		GB = 1024 * 1024 * 1024
	)
	size, err := strconv.Atoi(strSize)
	if err != nil {
		size = 0
	}
	if size >= GB {
		return fmt.Sprintf("%.2f GB", float64(size/GB))
	}
	return fmt.Sprintf("%.2f MB", float64(size/MB))
}

func (t *TorrentProvider) parseTorrentTitle(title string) (*parsetorrentname.TorrentInfo, error) {
	info, err := parsetorrentname.Parse(title)
	if err != nil {
		return nil, err
	}
	return info, nil
}

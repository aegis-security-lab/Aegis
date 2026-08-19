package control

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/projectdiscovery/uncover"
	"github.com/projectdiscovery/uncover/sources"
	"gorm.io/gorm"
)

const (
	defaultUncoverLimit   = 100
	maxUncoverLimit       = 1000
	defaultUncoverTimeout = 60
	maxUncoverTimeout     = 24 * 60 * 60
	maxUncoverQueryLength = 10_000
	maxUncoverPreview     = 100
)

type UncoverEngine struct {
	ID               string                   `json:"id"`
	Name             string                   `json:"name"`
	Configured       bool                     `json:"configured"`
	Anonymous        bool                     `json:"anonymous"`
	CredentialFields []UncoverCredentialField `json:"credentialFields"`
	DocsURL          string                   `json:"docsUrl"`
	Example          string                   `json:"example"`
}

type UncoverCredentialField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Secret      bool   `json:"secret"`
	Configured  bool   `json:"configured"`
	MaskedValue string `json:"maskedValue,omitempty"`
}

type UncoverStatus struct {
	Engines    []UncoverEngine `json:"engines"`
	Formats    []string        `json:"formats"`
	TextFields []string        `json:"textFields"`
}

type SaveUncoverProviderInput struct {
	Values map[string]string `json:"values"`
}

type UncoverSearchInput struct {
	Engine  string `json:"engine"`
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
	Format  string `json:"format"`
	Field   string `json:"field"`
	Timeout int    `json:"timeout"`
}

type UncoverAsset struct {
	Timestamp int64  `json:"timestamp"`
	Source    string `json:"source"`
	IP        string `json:"ip"`
	Port      int    `json:"port"`
	Host      string `json:"host"`
	URL       string `json:"url"`
}

type UncoverExportInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Format      string `json:"format"`
	Field       string `json:"field,omitempty"`
	MimeType    string `json:"mimeType"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"downloadUrl"`
}

type UncoverSearchResult struct {
	ID             string             `json:"id"`
	Engine         string             `json:"engine"`
	Query          string             `json:"query"`
	Count          int                `json:"count"`
	Preview        []UncoverAsset     `json:"preview"`
	PreviewLimited bool               `json:"previewLimited"`
	Warnings       []string           `json:"warnings"`
	Export         UncoverExportInfo  `json:"export"`
	Attachment     *UncoverExportInfo `json:"attachment,omitempty"`
	DurationMS     int64              `json:"durationMs"`
	CreatedAt      time.Time          `json:"createdAt"`
}

type uncoverProviderRecord struct {
	Engine    string            `gorm:"primaryKey"`
	Values    map[string]string `gorm:"serializer:json;type:text"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

var uncoverExportID = regexp.MustCompile(`^uncover-[0-9]+-[0-9]+$`)

func uncoverTokenField(label, description string) []UncoverCredentialField {
	return []UncoverCredentialField{{Key: "apiKey", Label: label, Description: description, Secret: true}}
}

var uncoverEngineDefinitions = []UncoverEngine{
	{ID: "shodan", Name: "Shodan", CredentialFields: uncoverTokenField("API Key", "Shodan 账户的 API Key。"), DocsURL: "https://help.shodan.io/the-basics/search-query-fundamentals", Example: `product:nginx country:SG`},
	{ID: "censys", Name: "Censys", CredentialFields: []UncoverCredentialField{{Key: "apiToken", Label: "Personal Access Token", Description: "Censys Platform Personal Access Token。", Secret: true}, {Key: "organizationId", Label: "Organization ID", Description: "该 Token 所属的 Censys Organization ID。"}}, DocsURL: "https://docs.censys.com/docs/censys-query-language", Example: `host.services:(protocol=SSH and not port=22)`},
	{ID: "fofa", Name: "FOFA", CredentialFields: []UncoverCredentialField{{Key: "email", Label: "账户邮箱", Description: "FOFA 账户绑定邮箱。"}, {Key: "apiKey", Label: "API Key", Description: "FOFA 账户的 API Key。", Secret: true}}, DocsURL: "https://en.fofa.info/api/info", Example: `domain="example.com" && port="443"`},
	{ID: "shodan-idb", Name: "Shodan InternetDB", Anonymous: true, DocsURL: "https://internetdb.shodan.io/docs", Example: `1.1.1.1`},
	{ID: "quake", Name: "Quake", CredentialFields: uncoverTokenField("Token", "360 Quake API Token。"), DocsURL: "https://quake.360.net/quake/#/help", Example: `service:http AND country:CN`},
	{ID: "hunter", Name: "Hunter", CredentialFields: uncoverTokenField("API Key", "奇安信 Hunter API Key。"), DocsURL: "https://hunter.qianxin.com/home/helpCenter", Example: `domain="example.com"`},
	{ID: "zoomeye", Name: "ZoomEye", CredentialFields: uncoverTokenField("API Key", "ZoomEye API Key。"), DocsURL: "https://www.zoomeye.ai/help", Example: `service="ssh" && country="CN"`},
	{ID: "netlas", Name: "Netlas", CredentialFields: uncoverTokenField("API Key", "Netlas API Key。"), DocsURL: "https://docs.netlas.io/knowledge-base/query-language/", Example: `port:443 AND geo.country:US`},
	{ID: "criminalip", Name: "Criminal IP", CredentialFields: uncoverTokenField("API Key", "Criminal IP API Key。"), DocsURL: "https://search.criminalip.io/developer/sample-code", Example: `product:redis country:KR`},
	{ID: "publicwww", Name: "PublicWWW", CredentialFields: uncoverTokenField("API Key", "PublicWWW API Key。"), DocsURL: "https://publicwww.com/websites/", Example: `"generator" "WordPress"`},
	{ID: "hunterhow", Name: "Hunter.how", CredentialFields: uncoverTokenField("API Key", "Hunter.how API Key。"), DocsURL: "https://hunter.how/search-api", Example: `ip="1.1.1.1"`},
	{ID: "google", Name: "Google CSE", CredentialFields: []UncoverCredentialField{{Key: "apiKey", Label: "API Key", Description: "Google Custom Search JSON API Key。", Secret: true}, {Key: "searchEngineId", Label: "Search Engine ID", Description: "Programmable Search Engine 的 cx 标识。"}}, DocsURL: "https://developers.google.com/custom-search/v1/overview", Example: `site:example.com inurl:admin`},
	{ID: "odin", Name: "Odin", CredentialFields: uncoverTokenField("API Key", "Odin API Key。"), DocsURL: "https://docs.getodin.com/", Example: `services.port:443`},
	{ID: "binaryedge", Name: "BinaryEdge", CredentialFields: uncoverTokenField("API Key", "BinaryEdge API Key。"), DocsURL: "https://docs.binaryedge.io/api-v2.html", Example: `port:443 country:US`},
	{ID: "onyphe", Name: "ONYPHE", CredentialFields: uncoverTokenField("API Key", "ONYPHE API Key。"), DocsURL: "https://search.onyphe.io/help", Example: `category:datascan protocol:http`},
	{ID: "driftnet", Name: "Driftnet", CredentialFields: uncoverTokenField("API Key", "Driftnet API Key。"), DocsURL: "https://docs.driftnet.io/", Example: `domain:example.com`},
	{ID: "greynoise", Name: "GreyNoise", CredentialFields: uncoverTokenField("API Key", "GreyNoise API Key。"), DocsURL: "https://docs.greynoise.io/docs/gnql-query-language", Example: `classification:malicious last_seen:1d`},
	{ID: "daydaymap", Name: "DayDayMap", CredentialFields: uncoverTokenField("API Key", "DayDayMap API Key。"), DocsURL: "https://www.daydaymap.com/", Example: `domain="example.com"`},
	{ID: "nerdydata", Name: "NerdyData", CredentialFields: uncoverTokenField("API Key", "NerdyData API Key。"), DocsURL: "https://nerdydata.com/developers", Example: `"example.com"`},
}

func (s *Store) UncoverStatus() (UncoverStatus, error) {
	provider, values, err := s.uncoverProvider()
	if err != nil {
		return UncoverStatus{}, err
	}
	engines := make([]UncoverEngine, len(uncoverEngineDefinitions))
	for index, definition := range uncoverEngineDefinitions {
		definition.CredentialFields = append([]UncoverCredentialField{}, definition.CredentialFields...)
		for fieldIndex := range definition.CredentialFields {
			field := &definition.CredentialFields[fieldIndex]
			field.Configured = strings.TrimSpace(values[definition.ID][field.Key]) != ""
			if field.Configured {
				field.MaskedValue = "••••••••"
			}
		}
		definition.Configured = definition.Anonymous || uncoverProviderConfigured(provider, definition.ID)
		engines[index] = definition
	}
	return UncoverStatus{Engines: engines, Formats: []string{"txt", "json", "jsonl", "csv"}, TextFields: []string{"ip:port", "host:port", "ip", "host", "port", "url"}}, nil
}

func (s *Store) SaveUncoverProvider(engine string, input SaveUncoverProviderInput) (UncoverEngine, error) {
	definition, err := uncoverEngineDefinition(engine)
	if err != nil {
		return UncoverEngine{}, err
	}
	if definition.Anonymous {
		return UncoverEngine{}, errors.New("该引擎无需配置凭据")
	}
	if input.Values == nil {
		return UncoverEngine{}, errors.New("凭据不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	var record uncoverProviderRecord
	found := s.db.First(&record, "engine = ?", definition.ID).Error
	if found != nil && !errors.Is(found, gorm.ErrRecordNotFound) {
		return UncoverEngine{}, fmt.Errorf("读取引擎凭据: %w", found)
	}
	if record.Values == nil {
		record.Values = make(map[string]string, len(definition.CredentialFields))
	}
	allowed := make(map[string]struct{}, len(definition.CredentialFields))
	for _, field := range definition.CredentialFields {
		allowed[field.Key] = struct{}{}
	}
	for key, raw := range input.Values {
		if _, exists := allowed[key]; !exists {
			return UncoverEngine{}, fmt.Errorf("不支持的凭据字段 %q", key)
		}
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if len(value) > 8192 {
			return UncoverEngine{}, fmt.Errorf("%s 不能超过 8192 个字节", key)
		}
		record.Values[key] = value
	}
	for _, field := range definition.CredentialFields {
		if strings.TrimSpace(record.Values[field.Key]) == "" {
			return UncoverEngine{}, fmt.Errorf("%s 不能为空", field.Label)
		}
	}
	now := time.Now()
	if record.Engine == "" {
		record.Engine = definition.ID
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	if err := s.db.Save(&record).Error; err != nil {
		return UncoverEngine{}, fmt.Errorf("保存引擎凭据: %w", err)
	}
	return uncoverEngineStatus(definition, record.Values), nil
}

func (s *Store) DeleteUncoverProvider(engine string) error {
	definition, err := uncoverEngineDefinition(engine)
	if err != nil {
		return err
	}
	if definition.Anonymous {
		return errors.New("该引擎无需配置凭据")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.db.Delete(&uncoverProviderRecord{}, "engine = ?", definition.ID).Error; err != nil {
		return fmt.Errorf("清除引擎凭据: %w", err)
	}
	return nil
}

func uncoverEngineDefinition(engine string) (UncoverEngine, error) {
	engine = strings.ToLower(strings.TrimSpace(engine))
	index := slices.IndexFunc(uncoverEngineDefinitions, func(item UncoverEngine) bool { return item.ID == engine })
	if index < 0 {
		return UncoverEngine{}, errors.New("不支持的网络空间搜索引擎")
	}
	definition := uncoverEngineDefinitions[index]
	definition.CredentialFields = append([]UncoverCredentialField{}, definition.CredentialFields...)
	return definition, nil
}

func uncoverEngineStatus(definition UncoverEngine, values map[string]string) UncoverEngine {
	definition.CredentialFields = append([]UncoverCredentialField{}, definition.CredentialFields...)
	configured := definition.Anonymous
	if !definition.Anonymous {
		configured = len(definition.CredentialFields) > 0
	}
	for index := range definition.CredentialFields {
		field := &definition.CredentialFields[index]
		field.Configured = strings.TrimSpace(values[field.Key]) != ""
		if field.Configured {
			field.MaskedValue = "••••••••"
		} else {
			configured = false
		}
	}
	definition.Configured = configured
	return definition
}

func (s *Store) uncoverProvider() (*sources.Provider, map[string]map[string]string, error) {
	var records []uncoverProviderRecord
	if err := s.db.Find(&records).Error; err != nil {
		return nil, nil, fmt.Errorf("读取空间引擎配置: %w", err)
	}
	provider := &sources.Provider{}
	values := make(map[string]map[string]string, len(records))
	for _, record := range records {
		copyValues := make(map[string]string, len(record.Values))
		for key, value := range record.Values {
			copyValues[key] = strings.TrimSpace(value)
		}
		values[record.Engine] = copyValues
		definition, definitionErr := uncoverEngineDefinition(record.Engine)
		if definitionErr == nil && uncoverEngineStatus(definition, copyValues).Configured {
			applyUncoverProviderValues(provider, record.Engine, copyValues)
		}
	}
	return provider, values, nil
}

func applyUncoverProviderValues(provider *sources.Provider, engine string, values map[string]string) {
	apiKey := values["apiKey"]
	switch engine {
	case "shodan":
		provider.Shodan = []string{apiKey}
	case "censys":
		provider.Censys = []string{values["apiToken"] + ":" + values["organizationId"]}
	case "fofa":
		provider.Fofa = []string{values["email"] + ":" + apiKey}
	case "quake":
		provider.Quake = []string{apiKey}
	case "hunter":
		provider.Hunter = []string{apiKey}
	case "zoomeye":
		provider.ZoomEye = []string{apiKey}
	case "netlas":
		provider.Netlas = []string{apiKey}
	case "criminalip":
		provider.CriminalIP = []string{apiKey}
	case "publicwww":
		provider.Publicwww = []string{apiKey}
	case "hunterhow":
		provider.HunterHow = []string{apiKey}
	case "google":
		provider.Google = []string{apiKey + ":" + values["searchEngineId"]}
	case "odin":
		provider.Odin = []string{apiKey}
	case "binaryedge":
		provider.BinaryEdge = []string{apiKey}
	case "onyphe":
		provider.Onyphe = []string{apiKey}
	case "driftnet":
		provider.Driftnet = []string{apiKey}
	case "greynoise":
		provider.GreyNoise = []string{apiKey}
	case "daydaymap":
		provider.Daydaymap = []string{apiKey}
	case "nerdydata":
		provider.NerdyData = []string{apiKey}
	}
}

func uncoverProviderConfigured(provider *sources.Provider, engine string) bool {
	switch engine {
	case "shodan":
		return len(provider.Shodan) > 0
	case "censys":
		return len(provider.Censys) > 0
	case "fofa":
		return len(provider.Fofa) > 0
	case "quake":
		return len(provider.Quake) > 0
	case "hunter":
		return len(provider.Hunter) > 0
	case "zoomeye":
		return len(provider.ZoomEye) > 0
	case "netlas":
		return len(provider.Netlas) > 0
	case "criminalip":
		return len(provider.CriminalIP) > 0
	case "publicwww":
		return len(provider.Publicwww) > 0
	case "hunterhow":
		return len(provider.HunterHow) > 0
	case "google":
		return len(provider.Google) > 0
	case "odin":
		return len(provider.Odin) > 0
	case "binaryedge":
		return len(provider.BinaryEdge) > 0
	case "onyphe":
		return len(provider.Onyphe) > 0
	case "driftnet":
		return len(provider.Driftnet) > 0
	case "greynoise":
		return len(provider.GreyNoise) > 0
	case "daydaymap":
		return len(provider.Daydaymap) > 0
	case "nerdydata":
		return len(provider.NerdyData) > 0
	default:
		return false
	}
}

func normalizeUncoverSearchInput(input UncoverSearchInput) (UncoverSearchInput, error) {
	input.Engine = strings.ToLower(strings.TrimSpace(input.Engine))
	input.Query = strings.TrimSpace(input.Query)
	input.Format = strings.ToLower(strings.TrimSpace(input.Format))
	input.Field = strings.ToLower(strings.TrimSpace(input.Field))
	if !slices.ContainsFunc(uncoverEngineDefinitions, func(engine UncoverEngine) bool { return engine.ID == input.Engine }) {
		return UncoverSearchInput{}, errors.New("不支持的网络空间搜索引擎")
	}
	if input.Query == "" {
		return UncoverSearchInput{}, errors.New("搜索语法不能为空")
	}
	if len(input.Query) > maxUncoverQueryLength {
		return UncoverSearchInput{}, fmt.Errorf("搜索语法不能超过 %d 个字节", maxUncoverQueryLength)
	}
	if input.Limit == 0 {
		input.Limit = defaultUncoverLimit
	}
	if input.Limit < 1 || input.Limit > maxUncoverLimit {
		return UncoverSearchInput{}, fmt.Errorf("结果上限必须在 1-%d 之间", maxUncoverLimit)
	}
	if input.Format == "" {
		input.Format = "jsonl"
	}
	if !slices.Contains([]string{"txt", "json", "jsonl", "csv"}, input.Format) {
		return UncoverSearchInput{}, errors.New("导出格式必须是 txt、json、jsonl 或 csv")
	}
	if input.Field == "" {
		input.Field = "ip:port"
	}
	if !slices.Contains([]string{"ip:port", "host:port", "ip", "host", "port", "url"}, input.Field) {
		return UncoverSearchInput{}, errors.New("TXT 字段必须是 ip:port、host:port、ip、host、port 或 url")
	}
	if input.Timeout == 0 {
		input.Timeout = defaultUncoverTimeout
	}
	if input.Timeout < 1 || input.Timeout > maxUncoverTimeout {
		return UncoverSearchInput{}, fmt.Errorf("超时时间必须在 1-%d 秒之间", maxUncoverTimeout)
	}
	return input, nil
}

func (s *Store) RunUncoverSearch(ctx context.Context, input UncoverSearchInput) (UncoverSearchResult, error) {
	input, err := normalizeUncoverSearchInput(input)
	if err != nil {
		return UncoverSearchResult{}, err
	}
	engine, err := uncoverEngineDefinition(input.Engine)
	if err != nil {
		return UncoverSearchResult{}, err
	}
	provider, _, err := s.uncoverProvider()
	if err != nil {
		return UncoverSearchResult{}, err
	}
	if !engine.Anonymous && !uncoverProviderConfigured(provider, engine.ID) {
		return UncoverSearchResult{}, fmt.Errorf("%s 尚未配置，请先在空间搜索页面保存凭据", engine.Name)
	}

	startedAt := time.Now()
	searchContext, cancel := context.WithTimeout(ctx, time.Duration(input.Timeout)*time.Second)
	defer cancel()
	service, err := uncover.New(&uncover.Options{
		Agents: []string{input.Engine}, Queries: []string{input.Query}, Limit: input.Limit,
		MaxRetry: 2, Timeout: input.Timeout,
	})
	if err != nil {
		return UncoverSearchResult{}, fmt.Errorf("初始化 uncover: %w", err)
	}
	service.Provider = provider
	service.Keys = provider.GetKeys()
	service.Session.Keys = &service.Keys
	assets := make([]UncoverAsset, 0, min(input.Limit, maxUncoverPreview))
	warnings := []string{}
	seen := make(map[string]struct{}, input.Limit)
	reachedLimit := false
	err = service.ExecuteWithCallback(searchContext, func(raw sources.Result) {
		if raw.Error != nil {
			warnings = appendUniqueString(warnings, sanitizeUncoverError(raw.Error.Error(), service.Keys))
			return
		}
		asset := UncoverAsset{
			Timestamp: raw.Timestamp, Source: raw.Source, IP: raw.IP,
			Port: raw.Port, Host: raw.Host, URL: raw.Url,
		}
		key := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s", asset.Source, asset.IP, asset.Port, asset.Host, asset.URL)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		assets = append(assets, asset)
		if len(assets) >= input.Limit {
			reachedLimit = true
			cancel()
		}
	})
	if err != nil {
		return UncoverSearchResult{}, fmt.Errorf("执行 uncover 搜索: %w", err)
	}
	if searchContext.Err() != nil && !reachedLimit {
		if errors.Is(searchContext.Err(), context.DeadlineExceeded) {
			return UncoverSearchResult{}, fmt.Errorf("搜索超过 %d 秒，请缩小查询范围或提高超时时间", input.Timeout)
		}
		return UncoverSearchResult{}, searchContext.Err()
	}
	if len(assets) == 0 && len(warnings) > 0 {
		return UncoverSearchResult{}, fmt.Errorf("搜索引擎返回错误: %s", warnings[0])
	}
	if len(assets) > input.Limit {
		assets = assets[:input.Limit]
	}

	exported, err := buildUncoverExport(input.Format, input.Field, assets)
	if err != nil {
		return UncoverSearchResult{}, err
	}
	id := nextID("uncover")
	filename := uncoverExportFilename(input.Engine, input.Format, startedAt)
	exportInfo, err := s.saveUncoverExport(id, filename, input.Format, input.Field, exported)
	if err != nil {
		return UncoverSearchResult{}, err
	}
	preview := append([]UncoverAsset{}, assets...)
	if len(preview) > maxUncoverPreview {
		preview = preview[:maxUncoverPreview]
	}
	return UncoverSearchResult{
		ID: id, Engine: input.Engine, Query: input.Query, Count: len(assets),
		Preview: preview, PreviewLimited: len(preview) < len(assets), Warnings: warnings,
		Export: exportInfo, DurationMS: time.Since(startedAt).Milliseconds(), CreatedAt: startedAt,
	}, nil
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

func sanitizeUncoverError(value string, keys sources.Keys) string {
	value = strings.TrimSpace(value)
	secrets := []string{
		keys.CensysToken, keys.CensysOrgId, keys.Shodan, keys.FofaEmail, keys.FofaKey,
		keys.QuakeToken, keys.HunterToken, keys.ZoomEyeToken, keys.NetlasToken,
		keys.CriminalIPToken, keys.PublicwwwToken, keys.HunterHowToken, keys.GoogleKey,
		keys.GoogleCX, keys.OdinToken, keys.BinaryEdgeToken, keys.OnypheKey,
		keys.DriftnetToken, keys.GreyNoiseKey, keys.Daydaymap, keys.NerdyDataToken,
	}
	for _, secret := range secrets {
		if secret = strings.TrimSpace(secret); secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return truncate(value, 1200)
}

func buildUncoverExport(format, field string, assets []UncoverAsset) ([]byte, error) {
	switch format {
	case "json":
		return json.MarshalIndent(assets, "", "  ")
	case "jsonl":
		var output bytes.Buffer
		encoder := json.NewEncoder(&output)
		encoder.SetEscapeHTML(false)
		for _, asset := range assets {
			if err := encoder.Encode(asset); err != nil {
				return nil, err
			}
		}
		return output.Bytes(), nil
	case "csv":
		var output bytes.Buffer
		writer := csv.NewWriter(&output)
		if err := writer.Write([]string{"timestamp", "source", "ip", "port", "host", "url"}); err != nil {
			return nil, err
		}
		for _, asset := range assets {
			timestamp := ""
			if asset.Timestamp > 0 {
				timestamp = time.Unix(asset.Timestamp, 0).UTC().Format(time.RFC3339)
			}
			if err := writer.Write([]string{timestamp, asset.Source, asset.IP, strconv.Itoa(asset.Port), asset.Host, asset.URL}); err != nil {
				return nil, err
			}
		}
		writer.Flush()
		return output.Bytes(), writer.Error()
	case "txt":
		var output strings.Builder
		for _, asset := range assets {
			value := uncoverTextValue(field, asset)
			if value == "" {
				continue
			}
			output.WriteString(value)
			output.WriteByte('\n')
		}
		return []byte(output.String()), nil
	default:
		return nil, errors.New("unsupported uncover export format")
	}
}

func uncoverTextValue(field string, asset UncoverAsset) string {
	switch field {
	case "ip:port":
		return uncoverHostPort(asset.IP, asset.Port)
	case "host:port":
		return uncoverHostPort(asset.Host, asset.Port)
	case "ip":
		return asset.IP
	case "host":
		return asset.Host
	case "port":
		if asset.Port > 0 {
			return strconv.Itoa(asset.Port)
		}
	case "url":
		return asset.URL
	}
	return ""
}

func uncoverHostPort(host string, port int) string {
	if strings.TrimSpace(host) == "" || port <= 0 {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func uncoverExportFilename(engine, format string, createdAt time.Time) string {
	return fmt.Sprintf("%s-%s.%s", engine, createdAt.UTC().Format("20060102T150405Z"), format)
}

func uncoverExportMimeType(format string) string {
	switch format {
	case "json", "jsonl":
		return "application/json; charset=utf-8"
	case "csv":
		return "text/csv; charset=utf-8"
	default:
		return "text/plain; charset=utf-8"
	}
}

func (s *Store) saveUncoverExport(id, filename, format, field string, data []byte) (UncoverExportInfo, error) {
	directory := filepath.Join(s.dataDir, "uncover", "exports", id)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return UncoverExportInfo{}, err
	}
	path := filepath.Join(directory, filepath.Base(filename))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		_ = os.RemoveAll(directory)
		return UncoverExportInfo{}, err
	}
	if format != "txt" {
		field = ""
	}
	return UncoverExportInfo{
		ID: id, Name: filepath.Base(filename), Format: format, Field: field,
		MimeType: uncoverExportMimeType(format), Size: int64(len(data)),
		DownloadURL: "/api/tools/uncover/exports/" + id,
	}, nil
}

func (s *Store) UncoverExportFile(id string) (UncoverExportInfo, *os.File, error) {
	id = strings.TrimSpace(id)
	if !uncoverExportID.MatchString(id) {
		return UncoverExportInfo{}, nil, errors.New("uncover export not found")
	}
	directory := filepath.Join(s.dataDir, "uncover", "exports", id)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return UncoverExportInfo{}, nil, errors.New("uncover export not found")
	}
	regular := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			regular = append(regular, entry)
		}
	}
	if len(regular) != 1 {
		return UncoverExportInfo{}, nil, errors.New("uncover export not found")
	}
	sort.Slice(regular, func(i, j int) bool { return regular[i].Name() < regular[j].Name() })
	path := filepath.Join(directory, regular[0].Name())
	root := filepath.Join(s.dataDir, "uncover", "exports")
	if !pathWithin(root, path) {
		return UncoverExportInfo{}, nil, errors.New("invalid uncover export path")
	}
	file, err := os.Open(path)
	if err != nil {
		return UncoverExportInfo{}, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return UncoverExportInfo{}, nil, err
	}
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(info.Name())), ".")
	mimeType := uncoverExportMimeType(extension)
	if detected := mime.TypeByExtension(filepath.Ext(info.Name())); detected != "" {
		mimeType = detected
	}
	return UncoverExportInfo{
		ID: id, Name: info.Name(), Format: extension, MimeType: mimeType,
		Size: info.Size(), DownloadURL: "/api/tools/uncover/exports/" + id,
	}, file, nil
}

func (m *Manager) UncoverExecutionSearch(ctx context.Context, executionID, token string, input UncoverSearchInput) (UncoverSearchResult, error) {
	session := m.getSession(executionID)
	if session == nil || token == "" || !secureEqual(token, session.controlToken) {
		return UncoverSearchResult{}, errors.New("invalid execution control token")
	}
	agent, err := m.store.GetAgent(session.agentID)
	if err != nil {
		return UncoverSearchResult{}, err
	}
	if !agent.Permissions.AllowNetwork {
		return UncoverSearchResult{}, errors.New("Agent permission boundary: network access is disabled")
	}
	if !slices.Contains(agent.Tools, "aegis_uncover_search") {
		return UncoverSearchResult{}, errors.New("Agent 未配置网络空间搜索工具")
	}
	issue, err := m.store.GetIssue(session.issueID)
	if err != nil {
		return UncoverSearchResult{}, err
	}
	result, err := m.store.RunUncoverSearch(ctx, input)
	if err != nil {
		return UncoverSearchResult{}, err
	}
	exportInfo, file, err := m.store.UncoverExportFile(result.ID)
	if err != nil {
		return UncoverSearchResult{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxAttachmentSize+1))
	if err != nil {
		return UncoverSearchResult{}, err
	}
	attachment, err := m.store.captureGeneratedAttachment(
		issue, executionID, "uncover/"+result.ID+"/"+exportInfo.Name,
		exportInfo.Name, fmt.Sprintf("%s 查询导出，共 %d 条资产", result.Engine, result.Count), data,
	)
	if err != nil {
		return UncoverSearchResult{}, err
	}
	result.Attachment = &UncoverExportInfo{
		ID: attachment.ID, Name: attachment.Name, Format: result.Export.Format,
		Field: result.Export.Field, MimeType: attachment.MimeType, Size: attachment.Size,
		DownloadURL: "/api/attachments/" + attachment.ID,
	}
	m.store.addEvent(executionID, issue.ID, "attachment", "网络空间检索结果已发布", attachment.Name)
	return result, nil
}

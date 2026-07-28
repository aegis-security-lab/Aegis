package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type tavilySearchResponse struct {
	Answer       string          `json:"answer"`
	Results      []WebSearchItem `json:"results"`
	ResponseTime float64         `json:"response_time"`
}

func normalizeWebSearchInput(input WebSearchInput) (WebSearchInput, error) {
	input.Query = strings.TrimSpace(input.Query)
	input.Topic = strings.TrimSpace(input.Topic)
	input.SearchDepth = strings.TrimSpace(input.SearchDepth)
	if input.Query == "" {
		return input, errors.New("搜索关键词不能为空")
	}
	if input.Topic == "" {
		input.Topic = "general"
	}
	if input.SearchDepth == "" {
		input.SearchDepth = "basic"
	}
	if input.Topic != "general" && input.Topic != "news" && input.Topic != "finance" {
		return input, errors.New("搜索主题必须是 general、news 或 finance")
	}
	if input.SearchDepth != "basic" && input.SearchDepth != "advanced" {
		return input, errors.New("搜索深度必须是 basic 或 advanced")
	}
	if input.MaxResults == 0 {
		input.MaxResults = 5
	}
	if input.MaxResults < 1 || input.MaxResults > 20 {
		return input, errors.New("搜索结果数量必须在 1–20 之间")
	}
	return input, nil
}

func executeWebSearch(ctx context.Context, config WebSearchConfig, input WebSearchInput) (WebSearchResult, error) {
	input, err := normalizeWebSearchInput(input)
	if err != nil {
		return WebSearchResult{}, err
	}
	if strings.TrimSpace(config.Engine) != "tavily" {
		return WebSearchResult{}, errors.New("当前仅支持 Tavily 搜索引擎")
	}
	if strings.TrimSpace(config.BaseURL) == "" || strings.TrimSpace(config.APIKey) == "" {
		return WebSearchResult{}, errors.New("Tavily 搜索地址或 API Key 尚未配置")
	}
	body, err := json.Marshal(map[string]any{
		"query": input.Query, "topic": input.Topic, "search_depth": input.SearchDepth,
		"include_answer": input.IncludeAnswer, "max_results": input.MaxResults,
	})
	if err != nil {
		return WebSearchResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.BaseURL, bytes.NewReader(body))
	if err != nil {
		return WebSearchResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.APIKey)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return WebSearchResult{}, fmt.Errorf("Tavily 请求失败: %w", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return WebSearchResult{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return WebSearchResult{}, fmt.Errorf("Tavily 返回 HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(payload)))
	}
	var decoded tavilySearchResponse
	if err = json.Unmarshal(payload, &decoded); err != nil {
		return WebSearchResult{}, fmt.Errorf("解析 Tavily 响应失败: %w", err)
	}
	if decoded.Results == nil {
		decoded.Results = []WebSearchItem{}
	}
	return WebSearchResult{Engine: "tavily", Query: input.Query, Answer: decoded.Answer, Results: decoded.Results, ResponseTime: decoded.ResponseTime}, nil
}

func (s *Store) TestWebSearch(ctx context.Context, input WebSearchTestInput) (WebSearchResult, error) {
	config := input.Config
	stored := s.Config().WebSearch
	config.Engine = fallback(strings.TrimSpace(config.Engine), fallback(stored.Engine, "tavily"))
	config.BaseURL = fallback(strings.TrimSpace(config.BaseURL), fallback(stored.BaseURL, "https://api.tavily.com/search"))
	if strings.TrimSpace(config.APIKey) == "" {
		config.APIKey = stored.APIKey
	}
	return executeWebSearch(ctx, config, input.Search)
}

func (s *Store) SaveWebSearchConfig(input WebSearchConfig) (WebSearchConfigView, error) {
	input.Engine = fallback(strings.TrimSpace(input.Engine), "tavily")
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	if input.Engine != "tavily" {
		return WebSearchConfigView{}, errors.New("当前仅支持 Tavily 搜索引擎")
	}
	if input.BaseURL == "" {
		return WebSearchConfigView{}, errors.New("搜索服务 URL 不能为空")
	}
	if parsed, err := url.ParseRequestURI(input.BaseURL); err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return WebSearchConfigView{}, errors.New("搜索服务 URL 必须是有效的 HTTP 或 HTTPS 地址")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(input.APIKey) == "" {
		input.APIKey = s.config.WebSearch.APIKey
	}
	if input.APIKey == "" {
		return WebSearchConfigView{}, errors.New("Tavily API Key 不能为空")
	}
	input.Enabled = true
	now := time.Now()
	s.config.WebSearch = input
	s.config.UpdatedAt = now
	if err := s.db.Save(&configRecord{ID: 1, Value: s.config, UpdatedAt: now}).Error; err != nil {
		return WebSearchConfigView{}, err
	}
	s.updatedAt = now
	s.broadcastLocked()
	return WebSearchConfigView{Engine: input.Engine, BaseURL: input.BaseURL, HasAPIKey: true, Enabled: true}, nil
}

func (m *Manager) WebSearchFromExecution(ctx context.Context, executionID, token string, input WebSearchInput) (WebSearchResult, error) {
	if _, _, err := m.executionActor(executionID, token); err != nil {
		return WebSearchResult{}, err
	}
	return executeWebSearch(ctx, m.store.Config().WebSearch, input)
}

package control

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

type capturingKnowledgeRanker struct {
	candidates []KnowledgeCandidate
}

func (r *capturingKnowledgeRanker) Rank(_ context.Context, _ string, candidates []KnowledgeCandidate, _ int) (KnowledgeRankResult, error) {
	r.candidates = append([]KnowledgeCandidate{}, candidates...)
	return KnowledgeRankResult{
		Summary: "命中了支付规范。",
		Results: []KnowledgeRankedDocument{{
			DocumentID: candidates[0].DocumentID,
			Score:      0.92,
			Reason:     "关键词与问题匹配",
		}},
	}, nil
}

func TestKnowledgeStoreAndAgentAssociation(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	retrievalAgent, err := store.GetAgent("knowledge-retriever")
	if err != nil {
		t.Fatal(err)
	}
	if !retrievalAgent.Internal || !retrievalAgent.Builtin || len(retrievalAgent.Tools) != 0 || retrievalAgent.Permissions.AllowNetwork || retrievalAgent.Permissions.AllowShell || retrievalAgent.Permissions.AllowWrite {
		t.Fatalf("unexpected built-in retrieval agent boundary: %+v", retrievalAgent)
	}
	if _, err = store.executionAgent(retrievalAgent.ID); err == nil {
		t.Fatal("internal retrieval agent must not execute Issues")
	}
	base, err := store.CreateKnowledgeBase(SaveKnowledgeBaseInput{
		Name: "支付规范", Description: "支付域的接口与风控约束。", RetrievalProvider: KnowledgeProviderKeywordAI,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateKnowledgeDocument(base.ID, SaveKnowledgeDocumentInput{Name: "api.md", Content: "# API\n\n幂等键必须持久化。"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateKnowledgeDocument(base.ID, SaveKnowledgeDocumentInput{Name: "api.md", Content: "duplicate"}); err == nil {
		t.Fatal("expected duplicate document name to fail")
	}
	detail, err := store.GetKnowledgeBase(base.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.KnowledgeBase.DocumentCount != 1 || len(detail.Documents) != 1 {
		t.Fatalf("unexpected knowledge detail: %+v", detail)
	}

	agent, err := store.GetAgent("backend-engineer")
	if err != nil {
		t.Fatal(err)
	}
	input := SaveAgentInput{
		ID: agent.ID, Name: agent.Name, Description: agent.Description, Avatar: agent.Avatar,
		Category: agent.Category, Enabled: agent.Enabled, Model: agent.Model,
		SystemPrompt: agent.SystemPrompt, Tools: agent.Tools, SkillIDs: agent.SkillIDs,
		KnowledgeBaseIDs: []string{base.ID}, Permissions: agent.Permissions,
	}
	updated, err := store.UpdateAgent(agent.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.KnowledgeBaseIDs) != 1 || updated.KnowledgeBaseIDs[0] != base.ID {
		t.Fatalf("knowledge association was not persisted: %+v", updated.KnowledgeBaseIDs)
	}
	if err = store.DeleteKnowledgeBase(base.ID); err == nil {
		t.Fatal("expected associated knowledge base deletion to fail")
	}
	input.KnowledgeBaseIDs = nil
	if _, err = store.UpdateAgent(agent.ID, input); err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteKnowledgeBase(base.ID); err != nil {
		t.Fatal(err)
	}
}

func TestKeywordAIRetrieverCapsAIPreviewAtOneThousandRunes(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base, err := store.CreateKnowledgeBase(SaveKnowledgeBaseInput{Name: "架构", Description: "架构决策。", RetrievalProvider: KnowledgeProviderKeywordAI})
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("前", 1200) + " 支付幂等关键字"
	document, err := store.CreateKnowledgeDocument(base.ID, SaveKnowledgeDocumentInput{Name: "payments.md", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	ranker := &capturingKnowledgeRanker{}
	service := NewKnowledgeRetrievalService(store, NewKeywordAIRetriever(ranker))
	result, err := service.Search(context.Background(), []string{base.ID}, KnowledgeSearchInput{Query: "支付幂等", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(ranker.candidates) != 1 {
		t.Fatalf("expected one AI candidate, got %d", len(ranker.candidates))
	}
	if got := utf8.RuneCountInString(ranker.candidates[0].Preview); got != knowledgePreviewRunes {
		t.Fatalf("AI preview length=%d, want %d", got, knowledgePreviewRunes)
	}
	if ranker.candidates[0].KeywordScore == 0 {
		t.Fatal("keyword phase did not inspect the full document")
	}
	if len(result.Hits) != 1 || result.Hits[0].DocumentID != document.ID || result.Hits[0].Excerpt != ranker.candidates[0].Preview {
		t.Fatalf("unexpected search result: %+v", result)
	}
	if _, err = service.Search(context.Background(), []string{base.ID}, KnowledgeSearchInput{Query: "x", KnowledgeBaseID: "not-associated"}); err == nil {
		t.Fatal("expected unassociated knowledge base search to fail")
	}
}

func TestAgentKnowledgeRuntimeContext(t *testing.T) {
	base := KnowledgeBase{ID: "kb-1", Name: "产品知识", Description: "记录产品规则。"}
	prompt := agentKnowledgeSystemPrompt("base prompt", []KnowledgeBase{base})
	if !strings.Contains(prompt, base.ID) || !strings.Contains(prompt, base.Description) || !strings.Contains(prompt, "aegis_search_knowledge") {
		t.Fatalf("missing knowledge context: %s", prompt)
	}
	snapshot := snapshotTools([]string{"aegis_search_knowledge"})
	if len(snapshot) != 1 || snapshot[0].Source != "aegis_extension" || len(snapshot[0].Parameters) != 4 {
		t.Fatalf("unexpected tool snapshot: %+v", snapshot)
	}
	if snapshot[0].Parameters[0].Name != "description" || !snapshot[0].Parameters[0].Required {
		t.Fatalf("knowledge tool purpose parameter=%+v", snapshot[0].Parameters[0])
	}
}

func TestKnowledgeRetrievalProviderIsSelectedPerKnowledgeBase(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base, err := store.CreateKnowledgeBase(SaveKnowledgeBaseInput{Name: "向量知识", Description: "用于验证可替换检索后端。", RetrievalProvider: "llama_index"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateKnowledgeDocument(base.ID, SaveKnowledgeDocumentInput{Name: "index.md", Content: "# Index\n\nVector content."}); err != nil {
		t.Fatal(err)
	}
	service := NewKnowledgeRetrievalService(store, NewKeywordAIRetriever(&capturingKnowledgeRanker{}))
	if _, err = service.Search(context.Background(), []string{base.ID}, KnowledgeSearchInput{Query: "vector"}); err == nil || !strings.Contains(err.Error(), "llama_index") {
		t.Fatalf("expected an explicit unregistered provider error, got %v", err)
	}
}

package control

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/projectdiscovery/uncover/sources"
)

func TestNormalizeUncoverSearchInput(t *testing.T) {
	input, err := normalizeUncoverSearchInput(UncoverSearchInput{Engine: " ShOdAn ", Query: " product:nginx "})
	if err != nil {
		t.Fatal(err)
	}
	if input.Engine != "shodan" || input.Query != "product:nginx" || input.Limit != 100 || input.Format != "jsonl" || input.Field != "ip:port" || input.Timeout != 60 {
		t.Fatalf("unexpected normalized input: %+v", input)
	}
	invalid := []UncoverSearchInput{
		{Engine: "unknown", Query: "x"},
		{Engine: "shodan", Query: ""},
		{Engine: "shodan", Query: "x", Limit: 1001},
		{Engine: "shodan", Query: "x", Format: "xml"},
		{Engine: "shodan", Query: "x", Field: "raw"},
		{Engine: "shodan", Query: "x", Timeout: maxUncoverTimeout + 1},
	}
	for _, candidate := range invalid {
		if _, err = normalizeUncoverSearchInput(candidate); err == nil {
			t.Fatalf("expected validation error for %+v", candidate)
		}
	}
}

func TestBuildUncoverExports(t *testing.T) {
	assets := []UncoverAsset{{
		Timestamp: 1_700_000_000, Source: "shodan", IP: "2001:db8::1",
		Port: 443, Host: "app.example.com", URL: "https://app.example.com",
	}}
	textData, err := buildUncoverExport("txt", "ip:port", assets)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(textData); got != "[2001:db8::1]:443\n" {
		t.Fatalf("txt export=%q", got)
	}

	jsonData, err := buildUncoverExport("json", "", assets)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []UncoverAsset
	if err := json.Unmarshal(jsonData, &decoded); err != nil || len(decoded) != 1 || decoded[0].Host != assets[0].Host {
		t.Fatalf("json export invalid: %v %+v", err, decoded)
	}

	jsonlData, err := buildUncoverExport("jsonl", "", assets)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(jsonlData), "\n") != 1 || !strings.Contains(string(jsonlData), `"source":"shodan"`) {
		t.Fatalf("jsonl export=%q", string(jsonlData))
	}

	csvData, err := buildUncoverExport("csv", "", assets)
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(strings.NewReader(string(csvData))).ReadAll()
	if err != nil || len(records) != 2 || records[0][0] != "timestamp" || records[1][2] != assets[0].IP {
		t.Fatalf("csv export invalid: %v %+v", err, records)
	}
}

func TestUncoverExportFileRejectsTraversalAndServesSavedFile(t *testing.T) {
	store := &Store{dataDir: t.TempDir()}
	data := []byte("192.0.2.1:443\n")
	export, err := store.saveUncoverExport("uncover-1700000000000-1", "shodan-test.txt", "txt", "ip:port", data)
	if err != nil {
		t.Fatal(err)
	}
	if export.Field != "ip:port" || export.Size != int64(len(data)) {
		t.Fatalf("unexpected export metadata: %+v", export)
	}
	loaded, file, err := store.UncoverExportFile(export.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	contents, err := io.ReadAll(file)
	if err != nil || string(contents) != string(data) || loaded.Name != export.Name {
		t.Fatalf("loaded export invalid: %v %+v %q", err, loaded, contents)
	}
	if _, _, err = store.UncoverExportFile("../../aegis.db"); err == nil {
		t.Fatal("path traversal export id must be rejected")
	}
}

func TestGeneratedAttachmentIsDurableAndIdempotent(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Export assets", Priority: "medium", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("source,ip,port\nshodan,192.0.2.1,443\n")
	attachment, err := store.captureGeneratedAttachment(issue, "execution-test", "uncover/search-1/assets.csv", "assets.csv", "Search export", data)
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.captureGeneratedAttachment(issue, "execution-test", "uncover/search-1/assets.csv", "changed.csv", "Changed", []byte("changed"))
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != attachment.ID {
		t.Fatalf("generated attachment was not idempotent: %s != %s", again.ID, attachment.ID)
	}
	_, file, err := store.AttachmentFile(attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	contents, _ := io.ReadAll(file)
	if string(contents) != string(data) {
		t.Fatalf("attachment contents=%q", contents)
	}
}

func TestRedTeamAgentsIncludeUncoverToolAndSkill(t *testing.T) {
	store := configuredStore(t)
	for _, agentID := range []string{"red-team-lead", "red-team-engineer"} {
		agent, err := store.GetAgent(agentID)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(agent.Tools, uncoverToolID) || !slices.Contains(agent.SkillIDs, uncoverSkillID) {
			t.Fatalf("%s capabilities not connected: tools=%v skills=%v", agentID, agent.Tools, agent.SkillIDs)
		}
	}
	recon, err := store.GetAgent("recon-engineer")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(recon.Tools, uncoverToolID) || slices.Contains(recon.SkillIDs, uncoverSkillID) {
		t.Fatalf("recon Agent retained red-team-only uncover capability: tools=%v skills=%v", recon.Tools, recon.SkillIDs)
	}
	var skill SkillDefinition
	for _, candidate := range store.Skills() {
		if candidate.ID == "uncover-cyberspace-search" {
			skill = candidate
			break
		}
	}
	for _, expected := range []string{"ProjectDiscovery uncover", "### Shodan", "### Censys", "### FOFA", "### ZoomEye", "### Netlas", "aegis_uncover_search", "空间搜索 page"} {
		if !strings.Contains(skill.Content, expected) {
			t.Fatalf("uncover Skill missing %q", expected)
		}
	}
	for _, forbidden := range []string{"provider-config.yaml", "SHODAN_API_KEY"} {
		if strings.Contains(skill.Content, forbidden) {
			t.Fatalf("uncover Skill retained obsolete configuration instruction %q", forbidden)
		}
	}
}

func TestUncoverProviderConfigurationIsPersistedAndRedacted(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.UncoverStatus()
	if err != nil {
		t.Fatal(err)
	}
	shodan := status.Engines[slices.IndexFunc(status.Engines, func(engine UncoverEngine) bool { return engine.ID == "shodan" })]
	if shodan.Configured || len(shodan.CredentialFields) != 1 || shodan.CredentialFields[0].Configured {
		t.Fatalf("unexpected initial Shodan status: %+v", shodan)
	}

	const secret = "test-shodan-secret"
	saved, err := store.SaveUncoverProvider("shodan", SaveUncoverProviderInput{Values: map[string]string{"apiKey": secret}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Configured || strings.Contains(string(encoded), secret) || saved.CredentialFields[0].MaskedValue == "" {
		t.Fatalf("saved provider response leaked or omitted status: %s", encoded)
	}
	provider, _, err := store.uncoverProvider()
	if err != nil {
		t.Fatal(err)
	}
	if provider.GetKeys().Shodan != secret {
		t.Fatal("saved Shodan key was not injected into uncover")
	}
	database, _ := store.db.DB()
	_ = database.Close()

	reopened, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	status, err = reopened.UncoverStatus()
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(status)
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("status leaked persisted credential: %s", encoded)
	}
	shodan = status.Engines[slices.IndexFunc(status.Engines, func(engine UncoverEngine) bool { return engine.ID == "shodan" })]
	if !shodan.Configured {
		t.Fatal("persisted Shodan configuration was not restored")
	}
	if err := reopened.DeleteUncoverProvider("shodan"); err != nil {
		t.Fatal(err)
	}
	status, err = reopened.UncoverStatus()
	if err != nil {
		t.Fatal(err)
	}
	shodan = status.Engines[slices.IndexFunc(status.Engines, func(engine UncoverEngine) bool { return engine.ID == "shodan" })]
	if shodan.Configured {
		t.Fatal("deleted Shodan configuration remains active")
	}
}

func TestUncoverProviderConfigurationValidatesFields(t *testing.T) {
	store := configuredStore(t)
	if _, err := store.SaveUncoverProvider("censys", SaveUncoverProviderInput{Values: map[string]string{"apiToken": "token"}}); err == nil || !strings.Contains(err.Error(), "Organization ID") {
		t.Fatalf("expected missing paired credential error, got %v", err)
	}
	if _, err := store.SaveUncoverProvider("shodan", SaveUncoverProviderInput{Values: map[string]string{"unexpected": "secret"}}); err == nil {
		t.Fatal("unknown credential field was accepted")
	}
	if _, err := store.SaveUncoverProvider("shodan-idb", SaveUncoverProviderInput{Values: map[string]string{"apiKey": "secret"}}); err == nil {
		t.Fatal("anonymous provider accepted credentials")
	}
}

func TestUncoverProviderConfigurationIgnoresExternalFilesAndEnvironment(t *testing.T) {
	originalLocation := sources.DefaultProviderConfigLocation
	sources.DefaultProviderConfigLocation = filepath.Join(t.TempDir(), "provider-config.yaml")
	t.Cleanup(func() { sources.DefaultProviderConfigLocation = originalLocation })
	if err := os.WriteFile(sources.DefaultProviderConfigLocation, []byte("shodan:\n  - legacy-file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHODAN_API_KEY", "legacy-environment-secret")
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.UncoverStatus()
	if err != nil {
		t.Fatal(err)
	}
	shodan := status.Engines[slices.IndexFunc(status.Engines, func(engine UncoverEngine) bool { return engine.ID == "shodan" })]
	if shodan.Configured {
		t.Fatal("external file or environment credentials were used instead of the UI-managed database")
	}
}

func TestUncoverCapabilitySeedMigrationIsAppliedOnce(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.db.Delete(&registrySeedMigrationRecord{}, "id = ?", uncoverRedTeamSeedMigrationID).Error; err != nil {
		t.Fatal(err)
	}
	for _, agentID := range []string{"red-team-lead", "red-team-engineer", "recon-engineer"} {
		var record agentRecord
		if err := store.db.First(&record, "id = ?", agentID).Error; err != nil {
			t.Fatal(err)
		}
		if agentID == "recon-engineer" {
			record.Definition.Tools = uniqueStrings(append(record.Definition.Tools, uncoverToolID))
			record.Definition.SkillIDs = uniqueStrings(append(record.Definition.SkillIDs, uncoverSkillID))
		} else {
			record.Definition.Tools = stringsWithout(record.Definition.Tools, uncoverToolID)
			record.Definition.SkillIDs = stringsWithout(record.Definition.SkillIDs, uncoverSkillID)
		}
		if err := store.db.Save(&record).Error; err != nil {
			t.Fatal(err)
		}
	}
	database, _ := store.db.DB()
	_ = database.Close()

	reopened, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, agentID := range []string{"red-team-lead", "red-team-engineer"} {
		agent, getErr := reopened.GetAgent(agentID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if !slices.Contains(agent.Tools, uncoverToolID) || !slices.Contains(agent.SkillIDs, uncoverSkillID) {
			t.Fatalf("migration did not add uncover capability to %s: %+v", agentID, agent)
		}
	}
	recon, err := reopened.GetAgent("recon-engineer")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(recon.Tools, uncoverToolID) || slices.Contains(recon.SkillIDs, uncoverSkillID) {
		t.Fatalf("migration did not remove uncover capability from recon Agent: %+v", recon)
	}

	var leadRecord agentRecord
	if err := reopened.db.First(&leadRecord, "id = ?", "red-team-lead").Error; err != nil {
		t.Fatal(err)
	}
	leadRecord.Definition.Tools = stringsWithout(leadRecord.Definition.Tools, uncoverToolID)
	leadRecord.Definition.SkillIDs = stringsWithout(leadRecord.Definition.SkillIDs, uncoverSkillID)
	if err := reopened.db.Save(&leadRecord).Error; err != nil {
		t.Fatal(err)
	}
	database, _ = reopened.db.DB()
	_ = database.Close()

	reopenedAgain, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	lead, err := reopenedAgain.GetAgent("red-team-lead")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(lead.Tools, uncoverToolID) || slices.Contains(lead.SkillIDs, uncoverSkillID) {
		t.Fatalf("completed seed migration overwrote a later Agent customization: %+v", lead)
	}
}

func TestSanitizeUncoverErrorRedactsProviderCredentials(t *testing.T) {
	keys := sources.Keys{Shodan: "secret-shodan-key", FofaEmail: "operator@example.com", FofaKey: "secret-fofa-key"}
	message := sanitizeUncoverError("request key=secret-shodan-key email=operator@example.com token=secret-fofa-key", keys)
	if strings.Contains(message, keys.Shodan) || strings.Contains(message, keys.FofaEmail) || strings.Contains(message, keys.FofaKey) {
		t.Fatalf("credentials leaked in error: %s", message)
	}
}

func TestUncoverShodanInternetDBLive(t *testing.T) {
	if os.Getenv("AEGIS_LIVE_UNCOVER") != "1" {
		t.Skip("set AEGIS_LIVE_UNCOVER=1 to run the live ProjectDiscovery uncover smoke test")
	}
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	result, err := store.RunUncoverSearch(ctx, UncoverSearchInput{
		Engine: "shodan-idb", Query: "1.1.1.1", Limit: 5, Format: "jsonl", Timeout: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Count == 0 || result.Export.Size == 0 {
		t.Fatalf("live search returned no export: %+v", result)
	}
}

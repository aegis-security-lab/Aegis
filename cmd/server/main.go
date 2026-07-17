package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"aegis/internal/control"
	"github.com/gin-gonic/gin"
)

func main() {
	store, err := control.NewStore(envOr("AEGIS_DATA_DIR", "data"))
	if err != nil {
		log.Fatal(err)
	}
	manager, err := control.NewManager(store)
	if err != nil {
		log.Fatal(err)
	}
	defer manager.Close()
	cfg := store.Config()
	store.SetRuntimeProbe(control.DetectRuntime(cfg.NodePath, cfg.PiPath))
	router := buildRouter(store, manager, envOr("AEGIS_DIST", "dist"))
	server := &http.Server{Addr: ":" + envOr("PORT", "8080"), Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		log.Printf("Aegis listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
}

func buildRouter(store *control.Store, manager *control.Manager, dist string) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	_ = r.SetTrustedProxies(nil)
	api := r.Group("/api")
	api.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok", "time": time.Now()}) })
	api.GET("/state", func(c *gin.Context) { c.JSON(200, store.State()) })
	api.GET("/events", func(c *gin.Context) { streamState(c, store) })
	api.GET("/issues/:id", func(c *gin.Context) {
		v, err := store.GetIssueDetail(c.Param("id"))
		if err != nil {
			writeError(c, 404, err)
			return
		}
		c.JSON(200, v)
	})
	api.POST("/issues", func(c *gin.Context) {
		var in control.CreateIssueInput
		if !bindJSON(c, &in) {
			return
		}
		v, err := manager.CreateIssue(in)
		if err != nil {
			writeError(c, 422, err)
			return
		}
		c.JSON(201, v)
	})
	api.PATCH("/issues/:id", func(c *gin.Context) {
		var in control.UpdateIssueInput
		if !bindJSON(c, &in) {
			return
		}
		v, err := store.UpdateIssue(c.Param("id"), in)
		if err != nil {
			writeError(c, 422, err)
			return
		}
		manager.ReconcileIssue(v)
		c.JSON(200, v)
	})
	api.POST("/internal/executions/:id/decompose", func(c *gin.Context) {
		var in control.DecomposeIssueInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.DecomposeExecution(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusConflict, err)
			return
		}
		c.JSON(http.StatusCreated, result)
	})
	api.POST("/internal/executions/:id/attachments", func(c *gin.Context) {
		var in control.PublishAttachmentInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.PublishExecutionAttachment(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusCreated, result)
	})
	api.POST("/internal/executions/:id/knowledge/search", func(c *gin.Context) {
		var in control.KnowledgeSearchInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.SearchExecutionKnowledge(c.Request.Context(), c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.GET("/attachments/:id", func(c *gin.Context) {
		attachment, file, err := store.AttachmentFile(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		disposition := mime.FormatMediaType("attachment", map[string]string{"filename": attachment.Name})
		c.DataFromReader(http.StatusOK, info.Size(), attachment.MimeType, file, map[string]string{
			"Content-Disposition":    disposition,
			"X-Content-Type-Options": "nosniff",
		})
	})
	api.POST("/issues/:id/checkout", func(c *gin.Context) {
		var in control.CheckoutIssueInput
		if !bindJSON(c, &in) {
			return
		}
		v, err := store.CheckoutIssue(c.Param("id"), in)
		if err != nil {
			writeError(c, 409, err)
			return
		}
		c.JSON(200, v)
	})
	api.POST("/issues/:id/dispatch", func(c *gin.Context) {
		if err := manager.DispatchIssue(c.Param("id")); err != nil {
			writeError(c, 409, err)
			return
		}
		c.JSON(202, gin.H{"accepted": true})
	})
	api.POST("/tasks/:id/cancel", func(c *gin.Context) {
		var in control.CancelTaskInput
		if c.Request.ContentLength > 0 && !bindJSON(c, &in) {
			return
		}
		result, err := manager.CancelTask(c.Param("id"), in.Reason)
		if err != nil {
			writeError(c, 409, err)
			return
		}
		c.JSON(200, result)
	})
	api.POST("/issues/:id/relations", func(c *gin.Context) {
		var in control.CreateRelationInput
		if !bindJSON(c, &in) {
			return
		}
		v, err := store.AddRelation(c.Param("id"), in)
		if err != nil {
			writeError(c, 422, err)
			return
		}
		c.JSON(201, v)
	})
	api.DELETE("/relations/:id", func(c *gin.Context) {
		if err := store.DeleteRelation(c.Param("id")); err != nil {
			writeError(c, 404, err)
			return
		}
		c.Status(204)
	})
	api.POST("/issues/:id/comments", func(c *gin.Context) {
		var in struct {
			Body string `json:"body"`
		}
		if !bindJSON(c, &in) {
			return
		}
		v, err := manager.AddIssueComment(c.Param("id"), in.Body)
		if err != nil {
			writeError(c, 422, err)
			return
		}
		c.JSON(201, v)
	})
	api.POST("/issues/:id/chat", func(c *gin.Context) {
		var in struct {
			ExecutionID string `json:"executionId"`
			Message     string `json:"message"`
		}
		if !bindJSON(c, &in) {
			return
		}
		v, err := manager.SendChat(c.Param("id"), in.ExecutionID, in.Message)
		if err != nil {
			writeError(c, 422, err)
			return
		}
		c.JSON(201, v)
	})
	api.POST("/executions/:id/stop", func(c *gin.Context) {
		if err := manager.StopExecution(c.Param("id")); err != nil {
			writeError(c, 409, err)
			return
		}
		c.Status(204)
	})
	api.POST("/approvals/:id", func(c *gin.Context) {
		var in struct {
			Approved bool `json:"approved"`
		}
		if !bindJSON(c, &in) {
			return
		}
		v, err := manager.ResolveApproval(c.Param("id"), in.Approved)
		if err != nil {
			writeError(c, 409, err)
			return
		}
		c.JSON(200, v)
	})
	api.GET("/findings", func(c *gin.Context) {
		category := c.Query("category")
		severity := c.Query("severity")
		page := 1
		pageSize := 20
		if p, err := parseIntQuery(c.Query("page"), 1); err == nil {
			page = p
		}
		if ps, err := parseIntQuery(c.Query("pageSize"), 20); err == nil {
			pageSize = ps
		}
		result, err := store.ListFindings(category, severity, page, pageSize)
		if err != nil {
			writeError(c, 422, err)
			return
		}
		c.JSON(200, result)
	})
	api.POST("/findings", func(c *gin.Context) {
		var in control.CreateFindingInput
		if !bindJSON(c, &in) {
			return
		}
		v, err := store.CreateFinding(in)
		if err != nil {
			writeError(c, 422, err)
			return
		}
		c.JSON(201, v)
	})
	api.GET("/findings/:id", func(c *gin.Context) {
		v, err := store.GetFinding(c.Param("id"))
		if err != nil {
			writeError(c, 404, err)
			return
		}
		c.JSON(200, v)
	})
	api.PATCH("/findings/:id", func(c *gin.Context) {
		var in control.UpdateFindingInput
		if !bindJSON(c, &in) {
			return
		}
		v, err := store.UpdateFinding(c.Param("id"), in)
		if err != nil {
			writeError(c, 422, err)
			return
		}
		c.JSON(200, v)
	})
	api.GET("/sessions", func(c *gin.Context) { c.JSON(200, store.Sessions()) })
	api.GET("/sessions/:id", func(c *gin.Context) {
		v, err := store.GetSession(c.Param("id"))
		if err != nil {
			writeError(c, 404, err)
			return
		}
		c.JSON(200, v)
	})

	api.GET("/agents", func(c *gin.Context) { c.JSON(200, store.Agents()) })
	api.POST("/agents", func(c *gin.Context) {
		var in control.SaveAgentInput
		if !bindJSON(c, &in) {
			return
		}
		v, err := store.CreateAgent(in)
		if err != nil {
			writeError(c, 422, err)
			return
		}
		c.JSON(201, v)
	})
	api.PUT("/agents/:id", func(c *gin.Context) {
		var in control.SaveAgentInput
		if !bindJSON(c, &in) {
			return
		}
		v, err := store.UpdateAgent(c.Param("id"), in)
		if err != nil {
			writeError(c, 422, err)
			return
		}
		c.JSON(200, v)
	})
	api.DELETE("/agents/:id", func(c *gin.Context) {
		if err := store.DeleteAgent(c.Param("id")); err != nil {
			writeError(c, 409, err)
			return
		}
		c.Status(204)
	})
	api.GET("/knowledge-bases", func(c *gin.Context) {
		result, err := store.ListKnowledgeBases()
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.GET("/knowledge-bases/:id", func(c *gin.Context) {
		result, err := store.GetKnowledgeBase(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.POST("/knowledge-bases", func(c *gin.Context) {
		var in control.SaveKnowledgeBaseInput
		if !bindJSON(c, &in) {
			return
		}
		result, err := store.CreateKnowledgeBase(in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusCreated, result)
	})
	api.PUT("/knowledge-bases/:id", func(c *gin.Context) {
		var in control.SaveKnowledgeBaseInput
		if !bindJSON(c, &in) {
			return
		}
		result, err := store.UpdateKnowledgeBase(c.Param("id"), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.DELETE("/knowledge-bases/:id", func(c *gin.Context) {
		if err := store.DeleteKnowledgeBase(c.Param("id")); err != nil {
			writeError(c, http.StatusConflict, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
	api.POST("/knowledge-bases/:id/documents", func(c *gin.Context) {
		var in control.SaveKnowledgeDocumentInput
		if !bindJSON(c, &in) {
			return
		}
		result, err := store.CreateKnowledgeDocument(c.Param("id"), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusCreated, result)
	})
	api.PUT("/knowledge-documents/:id", func(c *gin.Context) {
		var in control.SaveKnowledgeDocumentInput
		if !bindJSON(c, &in) {
			return
		}
		result, err := store.UpdateKnowledgeDocument(c.Param("id"), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.DELETE("/knowledge-documents/:id", func(c *gin.Context) {
		if err := store.DeleteKnowledgeDocument(c.Param("id")); err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
	api.GET("/skills", func(c *gin.Context) { c.JSON(200, store.Skills()) })
	api.POST("/skills", func(c *gin.Context) {
		var in control.SaveSkillInput
		if !bindJSON(c, &in) {
			return
		}
		v, err := store.CreateSkill(in)
		if err != nil {
			writeError(c, 422, err)
			return
		}
		c.JSON(201, v)
	})
	api.PUT("/skills/:id", func(c *gin.Context) {
		var in control.SaveSkillInput
		if !bindJSON(c, &in) {
			return
		}
		v, err := store.UpdateSkill(c.Param("id"), in)
		if err != nil {
			writeError(c, 422, err)
			return
		}
		c.JSON(200, v)
	})
	api.DELETE("/skills/:id", func(c *gin.Context) {
		if err := store.DeleteSkill(c.Param("id")); err != nil {
			writeError(c, 409, err)
			return
		}
		c.Status(204)
	})
	api.POST("/skills/import", func(c *gin.Context) {
		h, err := c.FormFile("file")
		if err != nil {
			writeError(c, 400, errors.New("请选择 .zip 或 SKILL.md 文件"))
			return
		}
		f, err := h.Open()
		if err != nil {
			writeError(c, 400, err)
			return
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, 2*1024*1024+1))
		if err != nil {
			writeError(c, 400, err)
			return
		}
		v, err := store.ImportSkill(h.Filename, data)
		if err != nil {
			writeError(c, 422, err)
			return
		}
		c.JSON(201, v)
	})
	api.POST("/skills/install", func(c *gin.Context) {
		var in struct {
			Source string `json:"source"`
		}
		if !bindJSON(c, &in) {
			return
		}
		v, err := store.InstallSkill(in.Source)
		if err != nil {
			writeError(c, 422, err)
			return
		}
		c.JSON(201, v)
	})
	api.GET("/skills/:id/export", func(c *gin.Context) {
		data, name, err := store.ExportSkill(c.Param("id"))
		if err != nil {
			writeError(c, 404, err)
			return
		}
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
		c.Data(200, "application/zip", data)
	})
	api.POST("/setup/probe", func(c *gin.Context) {
		var in struct {
			NodePath string `json:"nodePath"`
			PiPath   string `json:"piPath"`
		}
		if !bindJSON(c, &in) {
			return
		}
		p := control.DetectRuntime(strings.TrimSpace(in.NodePath), strings.TrimSpace(in.PiPath))
		store.SetRuntimeProbe(p)
		c.JSON(200, p)
	})
	api.POST("/setup/test", func(c *gin.Context) {
		var in control.SaveConfigInput
		if !bindJSON(c, &in) {
			return
		}
		if in.APIKey == "" {
			in.APIKey = store.Config().APIKey
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
		defer cancel()
		c.JSON(200, manager.TestConnection(ctx, in))
	})
	saveConfig := func(c *gin.Context) {
		var in control.SaveConfigInput
		if !bindJSON(c, &in) {
			return
		}
		if _, err := store.SaveConfig(in); err != nil {
			writeError(c, 422, err)
			return
		}
		cfg := store.Config()
		store.SetRuntimeProbe(control.DetectRuntime(cfg.NodePath, cfg.PiPath))
		c.JSON(200, store.State())
	}
	api.POST("/setup/complete", saveConfig)
	api.PUT("/settings", saveConfig)
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			writeError(c, 404, errors.New("endpoint not found"))
			return
		}
		requested := filepath.Join(dist, filepath.Clean(strings.TrimPrefix(c.Request.URL.Path, "/")))
		if info, err := os.Stat(requested); err == nil && !info.IsDir() {
			c.File(requested)
			return
		}
		index := filepath.Join(dist, "index.html")
		if _, err := os.Stat(index); err != nil {
			c.JSON(200, gin.H{"message": "Aegis API is running"})
			return
		}
		c.File(index)
	})
	return r
}

func streamState(c *gin.Context, s *control.Store) {
	updates, unsubscribe := s.Subscribe()
	defer unsubscribe()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	c.Stream(func(w io.Writer) bool {
		select {
		case state, ok := <-updates:
			if !ok {
				return false
			}
			c.SSEvent("state", state)
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}
func bindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		writeError(c, 400, fmt.Errorf("invalid JSON: %w", err))
		return false
	}
	return true
}
func writeError(c *gin.Context, status int, err error) {
	c.AbortWithStatusJSON(status, gin.H{"error": err.Error()})
}
func envOr(k, v string) string {
	if x := strings.TrimSpace(os.Getenv(k)); x != "" {
		return x
	}
	return v
}
func parseIntQuery(v string, defaultVal int) (int, error) {
	if strings.TrimSpace(v) == "" {
		return defaultVal, nil
	}
	i, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || i < 1 {
		return 0, errors.New("invalid integer")
	}
	return i, nil
}

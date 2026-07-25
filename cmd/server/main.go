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
	manager.BeginShutdown()
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
	api.GET("/container-profiles", func(c *gin.Context) { c.JSON(http.StatusOK, store.ContainerProfiles()) })
	api.GET("/containers", func(c *gin.Context) { c.JSON(http.StatusOK, store.Containers()) })
	api.POST("/container-profiles/image/build", func(c *gin.Context) {
		output, err := control.BuildWorkerContainerImage(c.Request.Context())
		if err != nil {
			writeError(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"image": control.WorkerContainerImage, "output": output})
	})
	api.POST("/containers/:id/start", func(c *gin.Context) {
		container, err := store.StartContainer(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, container)
	})
	api.POST("/containers/:id/stop", func(c *gin.Context) {
		container, err := store.StopContainer(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, container)
	})
	api.POST("/container-profiles", func(c *gin.Context) {
		var in control.SaveContainerProfileInput
		if !bindJSON(c, &in) {
			return
		}
		profile, err := store.SaveContainerProfile("", in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusCreated, profile)
	})
	api.PUT("/container-profiles/:id", func(c *gin.Context) {
		var in control.SaveContainerProfileInput
		if !bindJSON(c, &in) {
			return
		}
		profile, err := store.SaveContainerProfile(c.Param("id"), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, profile)
	})
	api.GET("/container-profiles/:id/delete-impact", func(c *gin.Context) {
		impact, err := store.ContainerProfileDeleteImpact(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		c.JSON(http.StatusOK, impact)
	})
	api.DELETE("/container-profiles/:id", func(c *gin.Context) {
		cascadeIssues := c.Query("cascadeIssues") == "true"
		result, err := manager.DeleteContainerProfile(c.Param("id"), cascadeIssues)
		if err != nil {
			status := http.StatusUnprocessableEntity
			if errors.Is(err, control.ErrContainerProfileReferenced) {
				status = http.StatusConflict
			}
			writeError(c, status, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.GET("/containers/:id/delete-impact", func(c *gin.Context) {
		impact, err := store.ContainerDeleteImpact(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		c.JSON(http.StatusOK, impact)
	})
	api.DELETE("/containers/:id", func(c *gin.Context) {
		result, err := manager.DeleteContainer(c.Param("id"), c.Query("cascadeIssues") == "true")
		if err != nil {
			status := http.StatusUnprocessableEntity
			if errors.Is(err, control.ErrContainerProfileReferenced) {
				status = http.StatusConflict
			}
			writeError(c, status, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.GET("/container-profiles/probe/docker", func(c *gin.Context) {
		if err := control.ProbeDocker(); err != nil {
			writeError(c, http.StatusServiceUnavailable, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ready": true})
	})
	api.GET("/state", func(c *gin.Context) { c.JSON(200, store.State()) })
	api.GET("/events", func(c *gin.Context) { streamState(c, store) })
	api.GET("/tools/uncover/status", func(c *gin.Context) {
		status, err := store.UncoverStatus()
		if err != nil {
			writeError(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, status)
	})
	api.PUT("/tools/uncover/providers/:engine", func(c *gin.Context) {
		var in control.SaveUncoverProviderInput
		if !bindJSON(c, &in) {
			return
		}
		engine, err := store.SaveUncoverProvider(c.Param("engine"), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, engine)
	})
	api.DELETE("/tools/uncover/providers/:engine", func(c *gin.Context) {
		if err := store.DeleteUncoverProvider(c.Param("engine")); err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
	api.POST("/tools/uncover/search", func(c *gin.Context) {
		var in control.UncoverSearchInput
		if !bindJSON(c, &in) {
			return
		}
		result, err := store.RunUncoverSearch(c.Request.Context(), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.GET("/tools/uncover/exports/:id", func(c *gin.Context) {
		export, file, err := store.UncoverExportFile(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		defer file.Close()
		disposition := mime.FormatMediaType("attachment", map[string]string{"filename": export.Name})
		c.DataFromReader(http.StatusOK, export.Size, export.MimeType, file, map[string]string{
			"Content-Disposition":    disposition,
			"X-Content-Type-Options": "nosniff",
		})
	})
	api.GET("/concierge/conversations", func(c *gin.Context) {
		items, err := store.ListConciergeConversations()
		if err != nil {
			writeError(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"conversations": items})
	})
	api.POST("/concierge/conversations", func(c *gin.Context) {
		conversation, err := manager.CreateConciergeConversation()
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusCreated, conversation)
	})
	api.GET("/concierge/conversations/:id", func(c *gin.Context) {
		detail, err := store.GetConciergeConversation(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		c.JSON(http.StatusOK, detail)
	})
	api.DELETE("/concierge/conversations/:id", func(c *gin.Context) {
		if err := manager.DeleteConciergeConversation(c.Param("id")); err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
	api.POST("/concierge/conversations/:id/messages", func(c *gin.Context) {
		var in struct {
			Message string `json:"message"`
		}
		if !bindJSON(c, &in) {
			return
		}
		message, err := manager.SendConciergeMessage(c.Param("id"), in.Message)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusCreated, message)
	})
	api.GET("/issues/:id", func(c *gin.Context) {
		v, err := store.GetIssueDetail(c.Param("id"))
		if err != nil {
			writeError(c, 404, err)
			return
		}
		c.JSON(200, v)
	})
	api.GET("/issues/:id/agents/:agentId/timeline", func(c *gin.Context) {
		result, err := store.IssueAgentTimeline(c.Param("id"), c.Param("agentId"))
		if err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.GET("/issues/:id/comments", func(c *gin.Context) {
		v, err := store.IssueCommentsPage(c.Param("id"), c.Query("before"), detailLimit(c))
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, v)
	})
	api.GET("/issues/:id/events", func(c *gin.Context) {
		v, err := store.IssueEventsPage(c.Param("id"), c.Query("before"), detailLimit(c))
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, v)
	})
	api.GET("/issues/:id/executions", func(c *gin.Context) {
		v, err := store.IssueExecutionsPage(c.Param("id"), c.Query("before"), detailLimit(c))
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, v)
	})
	api.GET("/execution-events/:id", func(c *gin.Context) {
		v, err := store.GetExecutionEvent(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		c.JSON(http.StatusOK, v)
	})
	api.GET("/tasks/:id/timeline", func(c *gin.Context) {
		v, err := store.TaskTimeline(c.Param("id"))
		if err != nil {
			writeError(c, 404, err)
			return
		}
		c.JSON(200, v)
	})
	api.GET("/tasks/:id/workspace", func(c *gin.Context) {
		v, err := store.TaskWorkspace(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, v)
	})
	api.POST("/tasks/:id/restart", func(c *gin.Context) {
		v, err := manager.RestartTask(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusCreated, v)
	})
	api.PATCH("/tasks/:id/budget", func(c *gin.Context) {
		var in struct {
			TimeBudgetMinutes *int `json:"timeBudgetMinutes"`
		}
		if !bindJSON(c, &in) {
			return
		}
		task, _, err := store.UpdateTaskBudget(c.Param("id"), in.TimeBudgetMinutes)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, task)
	})
	api.POST("/tasks", func(c *gin.Context) {
		var in control.CreateIssueInput
		if !bindJSON(c, &in) {
			return
		}
		task, issue, err := manager.CreateTask(in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusCreated, gin.H{"task": task, "issue": issue})
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
	api.POST("/internal/executions/:id/concierge/tasks", func(c *gin.Context) {
		var in control.CreateConciergeTaskInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		issue, err := manager.CreateTaskFromConcierge(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusConflict, err)
			return
		}
		c.JSON(http.StatusCreated, issue)
	})
	api.POST("/internal/executions/:id/attachments", func(c *gin.Context) {
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, control.MaxAttachmentSize+(1<<20))
		header, err := c.FormFile("file")
		if err != nil {
			writeError(c, http.StatusBadRequest, errors.New("附件文件不能为空或请求超过大小限制"))
			return
		}
		file, err := header.Open()
		if err != nil {
			writeError(c, http.StatusBadRequest, err)
			return
		}
		defer file.Close()
		name := strings.TrimSpace(c.PostForm("name"))
		if name == "" {
			name = header.Filename
		}
		in := control.PublishAttachmentInput{
			Path:        c.PostForm("path"),
			Name:        name,
			Description: c.PostForm("description"),
		}
		result, err := manager.UploadExecutionAttachment(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in, file, header.Size)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusCreated, result)
	})
	api.POST("/internal/executions/:id/final-result", func(c *gin.Context) {
		var in control.SubmitFinalResultInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.SubmitFinalResult(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.POST("/internal/executions/:id/uncover", func(c *gin.Context) {
		var in control.UncoverSearchInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.UncoverExecutionSearch(c.Request.Context(), c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.POST("/internal/executions/:id/progress", func(c *gin.Context) {
		var in control.ReportExecutionProgressInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.ReportExecutionProgress(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusCreated, result)
	})
	api.POST("/internal/executions/:id/broadcasts", func(c *gin.Context) {
		var in control.BroadcastMessageInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.BroadcastExecution(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusCreated, result)
	})
	api.GET("/internal/executions/:id/broadcasts", func(c *gin.Context) {
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		limit, err := strconv.Atoi(fallbackQuery(c.Query("limit"), "20"))
		if err != nil || limit < 1 || limit > 50 {
			writeError(c, http.StatusBadRequest, errors.New("invalid broadcast history limit"))
			return
		}
		result, err := manager.ExecutionBroadcastHistory(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), limit)
		if err != nil {
			writeError(c, http.StatusUnauthorized, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"broadcasts": result})
	})
	api.GET("/internal/executions/:id/child-issues", func(c *gin.Context) {
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		includeComments, err := strconv.ParseBool(fallbackQuery(c.Query("includeComments"), "true"))
		if err != nil {
			writeError(c, http.StatusBadRequest, errors.New("invalid includeComments value"))
			return
		}
		result, err := manager.ExecutionChildIssues(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), includeComments)
		if err != nil {
			writeError(c, http.StatusUnauthorized, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"children": result})
	})
	api.POST("/internal/executions/:id/child-waits", func(c *gin.Context) {
		var in control.WaitForChildIssuesInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.WaitForChildIssues(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusCreated, result)
	})
	api.POST("/internal/executions/:id/cancel-issue", func(c *gin.Context) {
		var in control.CancelIssueInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.CancelIssueFromExecution(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.POST("/internal/executions/:id/resume-issue-tree", func(c *gin.Context) {
		var in control.ResumeIssueTreeInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.ResumeIssueTreeFromExecution(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.POST("/internal/executions/:id/issue-progress", func(c *gin.Context) {
		var in control.GetIssueProgressInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := c.GetHeader("Authorization")
		result, err := manager.GetIssueProgress(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.POST("/internal/executions/:id/issue-comments", func(c *gin.Context) {
		var in control.CommentIssueInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.CommentIssueFromExecution(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusCreated, result)
	})
	api.GET("/internal/executions/:id/validation/attachments", func(c *gin.Context) {
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.ValidationAttachments(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")))
		if err != nil {
			writeError(c, http.StatusUnauthorized, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"attachments": result})
	})
	api.GET("/internal/executions/:id/validation/attachments/:attachmentId", func(c *gin.Context) {
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		offset, err := strconv.ParseInt(fallbackQuery(c.Query("offset"), "0"), 10, 64)
		if err != nil {
			writeError(c, http.StatusBadRequest, errors.New("invalid attachment offset"))
			return
		}
		limit, err := strconv.Atoi(fallbackQuery(c.Query("limit"), "16384"))
		if err != nil {
			writeError(c, http.StatusBadRequest, errors.New("invalid attachment limit"))
			return
		}
		result, err := manager.ReadValidationAttachment(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), c.Param("attachmentId"), offset, limit)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.POST("/internal/executions/:id/validation/decision", func(c *gin.Context) {
		var in control.SubmitValidationDecisionInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.SubmitValidationDecision(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.POST("/internal/executions/:id/validation/close", func(c *gin.Context) {
		var in control.CloseValidatedIssueInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.CloseValidatedIssue(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, result)
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
	api.GET("/internal/executions/:id/memo", func(c *gin.Context) {
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.ExecutionAgentMemo(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")))
		if err != nil {
			writeError(c, http.StatusUnauthorized, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.PUT("/internal/executions/:id/memo", func(c *gin.Context) {
		var in control.UpdateAgentMemoInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.UpdateExecutionAgentMemo(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.POST("/internal/executions/:id/rework", func(c *gin.Context) {
		var in control.RequestIssueReworkInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.RequestExecutionRework(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusAccepted, result)
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
	api.POST("/issues/:id/abandon", func(c *gin.Context) {
		var in control.AbandonIssueInput
		if c.Request.ContentLength > 0 && !bindJSON(c, &in) {
			return
		}
		issue, err := manager.AbandonIssue(c.Param("id"), in.Reason)
		if err != nil {
			writeError(c, http.StatusConflict, err)
			return
		}
		c.JSON(http.StatusOK, issue)
	})
	api.PUT("/issues/:id/validation", func(c *gin.Context) {
		var in control.IssueValidationControlInput
		if !bindJSON(c, &in) {
			return
		}
		issue, err := manager.SetIssueValidationDisabled(c.Param("id"), in.Disabled)
		if err != nil {
			writeError(c, http.StatusConflict, err)
			return
		}
		c.JSON(http.StatusOK, issue)
	})
	api.POST("/issues/:id/validation/manual-reject", func(c *gin.Context) {
		var in control.ManualValidationOverrideInput
		if !bindJSON(c, &in) {
			return
		}
		issue, err := manager.ManuallyRejectValidation(c.Param("id"), in.Reason)
		if err != nil {
			writeError(c, http.StatusConflict, err)
			return
		}
		c.JSON(http.StatusOK, issue)
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
			AgentID     string `json:"agentId"`
			Message     string `json:"message"`
		}
		if !bindJSON(c, &in) {
			return
		}
		v, err := manager.SendIssueChat(c.Param("id"), in.ExecutionID, in.AgentID, in.Message)
		if err != nil {
			writeError(c, 422, err)
			return
		}
		c.JSON(201, v)
	})
	api.POST("/issues/:id/chat-with-attachments", func(c *gin.Context) {
		executionID := strings.TrimSpace(c.PostForm("executionId"))
		message := strings.TrimSpace(c.PostForm("message"))
		form, err := c.MultipartForm()
		if err != nil {
			writeError(c, 422, err)
			return
		}
		files := form.File["attachments"]
		if len(files) > 10 {
			writeError(c, 422, errors.New("每次最多上传 10 个附件"))
			return
		}
		uploaded := make([]control.OperatorAttachment, 0, len(files))
		for _, header := range files {
			file, openErr := header.Open()
			if openErr != nil {
				writeError(c, 422, openErr)
				return
			}
			item, uploadErr := manager.UploadOperatorAttachment(c.Param("id"), executionID, header.Filename, file, header.Size)
			file.Close()
			if uploadErr != nil {
				writeError(c, 422, uploadErr)
				return
			}
			uploaded = append(uploaded, item)
		}
		if len(uploaded) > 0 {
			var manifest strings.Builder
			manifest.WriteString("\n\n以下附件已由用户直接上传到你的任务容器，可按容器内绝对路径读取：\n")
			for _, item := range uploaded {
				fmt.Fprintf(&manifest, "- %s: %s (%d bytes)\n", item.Name, item.Path, item.Size)
			}
			message += manifest.String()
		}
		v, sendErr := manager.SendChat(c.Param("id"), executionID, message)
		if sendErr != nil {
			writeError(c, 422, sendErr)
			return
		}
		c.JSON(201, gin.H{"message": v, "attachments": uploaded})
	})
	api.POST("/executions/:id/stop", func(c *gin.Context) {
		if err := manager.StopExecution(c.Param("id")); err != nil {
			writeError(c, 409, err)
			return
		}
		c.Status(204)
	})
	api.POST("/executions/:id/interrupt-tool", func(c *gin.Context) {
		result, err := manager.InterruptCurrentTool(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusConflict, err)
			return
		}
		c.JSON(http.StatusAccepted, result)
	})
	api.POST("/approvals/:id", func(c *gin.Context) {
		var in control.ApprovalDecisionInput
		if !bindJSON(c, &in) {
			return
		}
		v, err := manager.ResolveApproval(c.Param("id"), in.Approved, in.ReviewContent)
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
	api.GET("/sessions/:id/messages", func(c *gin.Context) {
		v, err := store.SessionMessagesPage(c.Param("id"), c.Query("before"), detailLimit(c))
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, v)
	})
	api.GET("/sessions/:id/events", func(c *gin.Context) {
		v, err := store.SessionEventsPage(c.Param("id"), c.Query("before"), detailLimit(c))
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, v)
	})
	api.GET("/sessions/:id/progress", func(c *gin.Context) {
		v, err := store.SessionProgressPage(c.Param("id"), c.Query("before"), detailLimit(c))
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, v)
	})
	api.GET("/sessions/:id/delta", func(c *gin.Context) {
		since, err := time.Parse(time.RFC3339Nano, c.Query("since"))
		if err != nil {
			writeError(c, http.StatusBadRequest, errors.New("invalid session delta watermark"))
			return
		}
		v, err := store.SessionDelta(c.Param("id"), since)
		if err != nil {
			writeError(c, http.StatusConflict, err)
			return
		}
		c.JSON(http.StatusOK, v)
	})

	api.GET("/agents", func(c *gin.Context) { c.JSON(200, store.Agents()) })
	api.GET("/departments", func(c *gin.Context) { c.JSON(http.StatusOK, store.Departments()) })
	api.POST("/departments", func(c *gin.Context) {
		var in control.SaveDepartmentInput
		if !bindJSON(c, &in) {
			return
		}
		v, err := store.SaveDepartment(in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusCreated, v)
	})
	api.PUT("/departments/:id", func(c *gin.Context) {
		var in control.SaveDepartmentInput
		if !bindJSON(c, &in) {
			return
		}
		in.ID = c.Param("id")
		v, err := store.SaveDepartment(in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, v)
	})
	api.DELETE("/departments/:id", func(c *gin.Context) {
		if err := store.DeleteDepartment(c.Param("id")); err != nil {
			writeError(c, http.StatusConflict, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
	api.GET("/agent-templates", func(c *gin.Context) { c.JSON(200, store.AgentTemplates()) })
	api.POST("/agent-templates", func(c *gin.Context) {
		var in control.SaveAgentTemplateInput
		if c.ShouldBindJSON(&in) != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid template input"})
			return
		}
		v, err := store.SaveAgentTemplate(in)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, control.ErrAgentTemplateIDConflict) {
				status = http.StatusConflict
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, v)
	})
	api.PATCH("/agent-templates/:id/hidden", func(c *gin.Context) {
		var in control.SetAgentTemplateHiddenInput
		if c.ShouldBindJSON(&in) != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid hidden input"})
			return
		}
		v, err := store.SetAgentTemplateHidden(c.Param("id"), in.Hidden)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, v)
	})
	api.PATCH("/agent-templates/:id/note", func(c *gin.Context) {
		var in control.UpdateAgentTemplateNoteInput
		if c.ShouldBindJSON(&in) != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid note input"})
			return
		}
		v, err := store.UpdateAgentTemplateNote(c.Param("id"), in.Note)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, v)
	})
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

func fallbackQuery(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
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

func detailLimit(c *gin.Context) int {
	limit, err := parseIntQuery(c.Query("limit"), 50)
	if err != nil {
		return 50
	}
	if limit > 100 {
		return 100
	}
	return limit
}

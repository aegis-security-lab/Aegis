package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"aegis/agentapp"
	phonecap "aegis/agentapp/agentcoreadapter"
	"aegis/capability"
	"aegis/coordination"
	"aegis/internal/control"
	"aegis/internal/webui"
	"aegis/observability"
	observabilitysqlite "aegis/observability/sqlitestore"
	"github.com/gin-gonic/gin"
)

var version = "dev"

func main() {
	port := flag.Int("port", envInt("PORT", 8080), "HTTP 服务端口")
	password := flag.String("password", strings.TrimSpace(os.Getenv("AEGIS_PASSWORD")), "Web 访问密码（也可使用 AEGIS_PASSWORD）")
	dataDir := flag.String("data-dir", envOr("AEGIS_DATA_DIR", "data"), "数据目录")
	showVersion := flag.Bool("version", false, "显示版本")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *port < 1 || *port > 65535 {
		log.Fatal("port must be between 1 and 65535")
	}
	auth, err := newAuthService(*password, 24*time.Hour)
	if err != nil {
		log.Fatal("必须通过 --password 或 AEGIS_PASSWORD 设置 Web 访问密码")
	}
	store, err := control.NewStore(*dataDir)
	if err != nil {
		log.Fatal(err)
	}
	systemObservability, err := control.EnableObservability(store, os.Stdout, envOr("AEGIS_LOG_LEVEL", "info"))
	if err != nil {
		log.Fatal(err)
	}
	manager, err := control.NewManager(store)
	if err != nil {
		log.Fatal(err)
	}
	defer manager.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	workerID := envOr("AEGIS_WORKER_ID", "control-worker")
	phoneURL := strings.TrimSpace(os.Getenv("AEGIS_AGENTAPP_URL"))
	hostOptions := control.NativeHostOptions{
		Sources: []control.NativeCapabilitySource{{
			Kind: capability.KindTool, Name: "validation", Source: control.NativeValidationSource{Manager: manager},
		}, {
			Kind: capability.KindTool, Name: "concierge", Source: control.NativeConciergeSource{Manager: manager},
		}},
	}
	var taskPhone control.TaskPhoneClient
	if phoneURL != "" {
		phoneClient := &agentapp.Client{BaseURL: phoneURL, Token: os.Getenv("AEGIS_AGENTAPP_TOKEN")}
		hostOptions.Phone = &phonecap.Source{Client: phoneClient, InstalledApps: []string{"aegis.board", "aegis.relay"}}
		taskPhone = phoneClient
	} else {
		_, phoneClient, phoneErr := control.NewControlAgentPhone(manager)
		if phoneErr != nil {
			log.Fatal(phoneErr)
		}
		hostOptions.Phone = &phonecap.Source{Client: phoneClient, InstalledApps: []string{"aegis.board", "aegis.relay"}}
		taskPhone = phoneClient
	}
	manager.SetAgentPhoneClient(taskPhone)
	host, err := control.NewNativeAgentHost(store, hostOptions)
	if err != nil {
		log.Fatal(err)
	}
	nativeDelivery := control.NewNativeSessionDelivery(manager)
	manager.SetNativeAgentRuntime(host, nativeDelivery)
	capabilityPlanner := &coordination.CapabilityPlanner{Catalog: host.Capabilities}
	issueRunner := control.NativeIssueRunner{Manager: manager, Host: host, Sessions: nativeDelivery}
	nativeCoordination := &control.NativeCoordinationRuntime{Manager: manager, Host: host, PhoneEnabled: true, Planner: capabilityPlanner}
	runner := control.CoordinationRunner{Issues: issueRunner, Subagents: nativeCoordination}
	coordinationMode := strings.TrimSpace(os.Getenv("AEGIS_COORDINATION_MODE"))
	if coordinationMode == "" {
		coordinationMode = "board_autonomy"
	}
	coordinationBridge, err := control.NewCoordinationBridge(manager, control.CoordinationBridgeOptions{
		WorkerID: workerID, DefaultMode: coordinationMode, Delivery: nativeDelivery,
		Subagents: nativeCoordination, Executor: runner, Planner: capabilityPlanner, PhoneEnabled: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	manager.SetCoordination(coordinationBridge)
	nativeCoordination.Bridge = coordinationBridge
	defer coordinationBridge.Close()
	router := buildRouterWithAuth(store, manager, envOr("AEGIS_DIST", "dist"), auth)
	// Input attachments can be multi-gigabyte audit images. Keep the header
	// timeout, but do not terminate a healthy streaming request after 30 seconds.
	server := &http.Server{Addr: fmt.Sprintf(":%d", *port), Handler: router, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 90 * time.Second}
	go func() {
		systemObservability.Logger.Info(ctx, "server.listen", slog.String("address", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	coordinationBridge.Start(ctx)
	<-ctx.Done()
	manager.BeginShutdown()
	coordinationBridge.Close()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
}

func observabilityMiddleware(store *control.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if requestID == "" {
			requestID = observability.NewID(12)
		}
		traceID := strings.TrimSpace(c.GetHeader("X-Trace-ID"))
		if traceID == "" {
			traceID = observability.NewID(16)
		}
		ctx := observability.WithScope(c.Request.Context(), observability.Scope{TraceID: traceID, RequestID: requestID, Component: "server.http"})
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Request-ID", requestID)
		c.Header("X-Trace-ID", traceID)
		logger := observability.Default()
		logger.Debug(ctx, "http.request.start", slog.String("method", c.Request.Method), slog.String("path", c.Request.URL.Path), slog.String("remote_ip", c.ClientIP()))
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		duration := time.Since(started)
		status := c.Writer.Status()
		attrs := []slog.Attr{slog.String("method", c.Request.Method), slog.String("route", route), slog.Int("status", status), slog.Int64("response_bytes", int64(c.Writer.Size())), slog.Float64("duration_ms", float64(duration.Microseconds())/1000)}
		if len(c.Errors) > 0 {
			attrs = append(attrs, slog.String("error", c.Errors.String()))
		}
		switch {
		case status >= 500:
			logger.Error(ctx, "http.request.end", attrs...)
		case status >= 400:
			logger.Warn(ctx, "http.request.end", attrs...)
		default:
			logger.Info(ctx, "http.request.end", attrs...)
		}
		if system := store.Observability(); system != nil {
			labels := observability.Labels{"method": c.Request.Method, "route": route, "status": strconv.Itoa(status)}
			system.Metrics.AddCounter("http_requests_total", 1, labels)
			system.Metrics.ObserveHistogram("http_request_duration_ms", float64(duration.Microseconds())/1000, observability.Labels{"method": c.Request.Method, "route": route})
		}
	}
}

func buildRouter(store *control.Store, manager *control.Manager, dist string) *gin.Engine {
	return buildRouterWithAuth(store, manager, dist, nil)
}

func buildRouterWithAuth(store *control.Store, manager *control.Manager, dist string, auth *authService) *gin.Engine {
	r := gin.New()
	r.Use(observabilityMiddleware(store), gin.Recovery())
	_ = r.SetTrustedProxies(nil)
	if auth != nil {
		r.POST("/auth/login", auth.login)
	}
	api := r.Group("/api")
	if auth != nil {
		api.Use(auth.middleware())
		api.POST("/auth/logout", auth.logout)
		api.GET("/auth/session", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	}
	api.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok", "time": time.Now()}) })
	api.GET("/observability/metrics", func(c *gin.Context) {
		if system := store.Observability(); system != nil {
			c.JSON(http.StatusOK, system.Metrics.Snapshot())
			return
		}
		c.JSON(http.StatusOK, observability.MetricSnapshot{Counters: map[string]float64{}, Gauges: map[string]float64{}, Histograms: map[string]observability.Distribution{}})
	})
	api.GET("/observability/logs", func(c *gin.Context) {
		system := store.Observability()
		if system == nil || system.Logs == nil {
			c.JSON(http.StatusOK, gin.H{"logs": []any{}})
			return
		}
		limit, parseErr := parseIntQuery(c.Query("limit"), 500)
		if parseErr != nil {
			writeError(c, http.StatusBadRequest, parseErr)
			return
		}
		if limit > 5000 {
			limit = 5000
		}
		logs, queryErr := system.Logs.Query(c.Request.Context(), observabilitysqlite.Filter{TaskID: strings.TrimSpace(c.Query("taskId")), CoordinationID: strings.TrimSpace(c.Query("coordinationId")), TraceID: strings.TrimSpace(c.Query("traceId")), Limit: limit})
		if queryErr != nil {
			writeError(c, http.StatusInternalServerError, queryErr)
			return
		}
		c.JSON(http.StatusOK, gin.H{"logs": logs})
	})
	if coordinationBridge := manager.Coordination(); coordinationBridge != nil {
		api.GET("/coordination/executions/:id", func(c *gin.Context) {
			execution, err := coordinationBridge.Execution(c.Request.Context(), c.Param("id"))
			if err != nil {
				writeError(c, http.StatusNotFound, err)
				return
			}
			c.JSON(http.StatusOK, execution)
		})
		api.GET("/coordination/executions/:id/events", func(c *gin.Context) {
			after, err := strconv.ParseUint(fallbackQuery(c.Query("after"), "0"), 10, 64)
			if err != nil {
				writeError(c, http.StatusBadRequest, errors.New("invalid event cursor"))
				return
			}
			limit, err := parseIntQuery(c.Query("limit"), 100)
			if err != nil {
				writeError(c, http.StatusBadRequest, err)
				return
			}
			if limit > 500 {
				limit = 500
			}
			events, err := coordinationBridge.ExecutionEvents(c.Request.Context(), c.Param("id"), after, limit)
			if err != nil {
				writeError(c, http.StatusInternalServerError, err)
				return
			}
			c.JSON(http.StatusOK, gin.H{"events": events})
		})
	}
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
	api.POST("/containers/batch/stop", func(c *gin.Context) {
		var in struct {
			ContainerIDs []string `json:"containerIds"`
		}
		if !bindJSON(c, &in) {
			return
		}
		result, err := store.StopContainers(in.ContainerIDs)
		if err != nil {
			writeError(c, http.StatusBadRequest, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.POST("/containers/batch/delete-impact", func(c *gin.Context) {
		var in struct {
			ContainerIDs []string `json:"containerIds"`
		}
		if !bindJSON(c, &in) {
			return
		}
		impact, err := store.ContainerBatchDeleteImpact(in.ContainerIDs)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, impact)
	})
	api.POST("/containers/batch/delete", func(c *gin.Context) {
		var in struct {
			ContainerIDs  []string `json:"containerIds"`
			CascadeIssues bool     `json:"cascadeIssues"`
		}
		if !bindJSON(c, &in) {
			return
		}
		result, err := manager.DeleteContainers(in.ContainerIDs, in.CascadeIssues)
		if err != nil {
			writeError(c, http.StatusBadRequest, err)
			return
		}
		c.JSON(http.StatusOK, result)
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
	api.POST("/concierge/conversations/:id/attachments", func(c *gin.Context) {
		receiveInputAttachment(c, store, "concierge", c.Param("id"))
	})
	api.POST("/concierge/conversations/:id/messages", func(c *gin.Context) {
		var in struct {
			Message       string   `json:"message"`
			AttachmentIDs []string `json:"attachmentIds"`
		}
		if !bindJSON(c, &in) {
			return
		}
		message, err := manager.SendConciergeMessage(c.Param("id"), in.Message, in.AttachmentIDs)
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
	api.GET("/coordination/modes", func(c *gin.Context) {
		bridge := manager.Coordination()
		if bridge == nil {
			c.JSON(http.StatusOK, gin.H{"enabled": false, "modes": []any{}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"enabled": true, "directSubagents": bridge.DirectSubagentsAvailable(), "modes": bridge.Modes()})
	})
	api.POST("/coordination/invoke", func(c *gin.Context) {
		bridge := manager.Coordination()
		if bridge == nil {
			writeError(c, http.StatusServiceUnavailable, errors.New("coordination runtime is disabled"))
			return
		}
		var input coordination.AgentInvocation
		if !bindJSON(c, &input) {
			return
		}
		receipt, err := bridge.InvokeAgent(c.Request.Context(), input)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusAccepted, receipt)
	})
	api.GET("/coordination/capabilities", func(c *gin.Context) {
		bridge := manager.Coordination()
		if bridge == nil {
			c.JSON(http.StatusOK, gin.H{"enabled": false, "capabilities": []any{}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"enabled": true, "capabilities": bridge.CapabilityCatalog()})
	})
	api.POST("/coordination/capabilities/plan", func(c *gin.Context) {
		bridge := manager.Coordination()
		if bridge == nil {
			writeError(c, http.StatusServiceUnavailable, errors.New("coordination runtime is disabled"))
			return
		}
		var input struct {
			IssueID string                 `json:"issueId"`
			Child   coordination.ChildWork `json:"child"`
		}
		if !bindJSON(c, &input) {
			return
		}
		decision, err := bridge.PreviewAgentCapabilities(c.Request.Context(), input.IssueID, input.Child)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, decision)
	})
	api.GET("/issues/:id/coordination", func(c *gin.Context) {
		bridge := manager.Coordination()
		if bridge == nil {
			writeError(c, http.StatusServiceUnavailable, errors.New("coordination runtime is disabled"))
			return
		}
		binding, err := bridge.BindingForIssue(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		c.JSON(http.StatusOK, binding)
	})
	api.PUT("/issues/:id/coordination", func(c *gin.Context) {
		bridge := manager.Coordination()
		if bridge == nil {
			writeError(c, http.StatusServiceUnavailable, errors.New("coordination runtime is disabled"))
			return
		}
		var input struct {
			Mode    string          `json:"mode"`
			Version string          `json:"version"`
			Config  json.RawMessage `json:"config"`
		}
		if !bindJSON(c, &input) {
			return
		}
		binding, err := bridge.BindIssue(c.Request.Context(), c.Param("id"), input.Mode, input.Version, input.Config)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, binding)
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
	api.GET("/tasks/:id", func(c *gin.Context) {
		v, err := store.GetTaskDetail(c.Param("id"))
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
	api.GET("/tasks/:id/phones", func(c *gin.Context) {
		phones, err := manager.TaskPhones(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"phones": phones})
	})
	api.GET("/tasks/:id/agents", func(c *gin.Context) {
		items, err := store.TaskAgents(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"agents": items})
	})
	api.GET("/tasks/:id/export", func(c *gin.Context) {
		includeArtifacts, err := parseBoolQuery(c.Query("includeArtifacts"), true)
		if err != nil {
			writeError(c, http.StatusBadRequest, err)
			return
		}
		redactSecrets, err := parseBoolQuery(c.Query("redactSecrets"), true)
		if err != nil {
			writeError(c, http.StatusBadRequest, err)
			return
		}
		temp, err := os.CreateTemp("", "aegis-task-evidence-*.zip")
		if err != nil {
			writeError(c, http.StatusInternalServerError, err)
			return
		}
		name := temp.Name()
		defer func() {
			_ = temp.Close()
			_ = os.Remove(name)
		}()
		_ = temp.Chmod(0o600)
		manifest, err := manager.WriteTaskEvidence(c.Request.Context(), c.Param("id"), temp, control.TaskExportOptions{IncludeArtifacts: includeArtifacts, RedactSecrets: redactSecrets})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, control.ErrTaskEvidenceNotFound) {
				status = http.StatusNotFound
			}
			writeError(c, status, err)
			return
		}
		info, err := temp.Stat()
		if err != nil {
			writeError(c, http.StatusInternalServerError, err)
			return
		}
		if _, err = temp.Seek(0, io.SeekStart); err != nil {
			writeError(c, http.StatusInternalServerError, err)
			return
		}
		filename := "aegis-task-" + archiveFilenamePart(c.Param("id")) + "-evidence.zip"
		disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
		c.DataFromReader(http.StatusOK, info.Size(), "application/zip", temp, map[string]string{
			"Content-Disposition":             disposition,
			"X-Content-Type-Options":          "nosniff",
			"X-Aegis-Evidence-Complete":       strconv.FormatBool(manifest.Complete),
			"X-Aegis-Evidence-Redacted":       strconv.FormatBool(manifest.RedactionEnabled),
			"X-Aegis-Evidence-Schema-Version": manifest.SchemaVersion,
		})
	})
	api.GET("/tasks/:id/audits", func(c *gin.Context) {
		items, err := manager.ListTaskAudits(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"audits": items})
	})
	api.POST("/tasks/:id/audits", func(c *gin.Context) {
		audit, err := manager.CreateTaskAudit(c.Request.Context(), c.Param("id"))
		if err != nil {
			status := http.StatusUnprocessableEntity
			if errors.Is(err, control.ErrTaskEvidenceNotFound) {
				status = http.StatusNotFound
			}
			writeError(c, status, err)
			return
		}
		c.JSON(http.StatusCreated, audit)
	})
	api.GET("/task-audits/:id", func(c *gin.Context) {
		audit, err := manager.GetTaskAudit(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		c.JSON(http.StatusOK, audit)
	})
	api.GET("/task-audits/:id/report", func(c *gin.Context) {
		audit, err := manager.GetTaskAudit(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		if strings.TrimSpace(audit.ReportMarkdown) == "" {
			writeError(c, http.StatusConflict, errors.New("审计报告尚未生成"))
			return
		}
		filename := "aegis-task-audit-" + archiveFilenamePart(audit.ID) + ".md"
		disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
		c.Header("Content-Disposition", disposition)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(audit.ReportMarkdown))
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
		task, err := store.UpdateTaskBudget(c.Param("id"), in.TimeBudgetMinutes)
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
	api.GET("/task-templates", func(c *gin.Context) {
		templates, err := store.TaskTemplates()
		if err != nil {
			writeError(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"templates": templates})
	})
	api.PUT("/task-templates", func(c *gin.Context) {
		var in control.TaskTemplate
		if !bindJSON(c, &in) {
			return
		}
		template, err := store.SaveTaskTemplate(in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, template)
	})
	api.DELETE("/task-templates/:id", func(c *gin.Context) {
		if err := store.DeleteTaskTemplate(c.Param("id")); err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
	api.POST("/tasks/attachments", func(c *gin.Context) {
		receiveInputAttachment(c, store, "task", "")
	})
	api.DELETE("/input-attachments/:id", func(c *gin.Context) {
		if err := store.DeleteStagedInputAttachment(c.Param("id")); err != nil {
			writeError(c, http.StatusConflict, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
	api.GET("/input-attachments/:id", func(c *gin.Context) {
		attachment, file, err := store.InputAttachmentFile(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || info.Size() != attachment.Size {
			writeError(c, http.StatusNotFound, errors.New("输入附件在服务端不存在或不完整"))
			return
		}
		disposition := mime.FormatMediaType("attachment", map[string]string{"filename": attachment.Name})
		c.DataFromReader(http.StatusOK, info.Size(), attachment.MimeType, file, map[string]string{
			"Content-Disposition": disposition, "X-Content-Type-Options": "nosniff",
		})
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
		v, err := manager.UpdateBoardIssue(c.Param("id"), in)
		if err != nil {
			writeError(c, 422, err)
			return
		}
		c.JSON(200, v)
	})
	api.DELETE("/issues/:id", func(c *gin.Context) {
		result, err := manager.DeleteBoardIssue(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusConflict, err)
			return
		}
		c.JSON(http.StatusOK, result)
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
	api.POST("/internal/executions/:id/board", func(c *gin.Context) {
		var in control.BoardCommandInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.BoardCommandFromExecution(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.POST("/internal/executions/:id/relay", func(c *gin.Context) {
		var in control.RelayCommandInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.RelayCommandFromExecution(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.POST("/internal/executions/:id/web-search", func(c *gin.Context) {
		var in control.WebSearchInput
		if !bindJSON(c, &in) {
			return
		}
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(c, http.StatusUnauthorized, errors.New("missing execution control token"))
			return
		}
		result, err := manager.WebSearchFromExecution(c.Request.Context(), c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), in)
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
		result, err := manager.ReadValidationAttachment(c.Param("id"), strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")), c.Param("attachmentId"), c.Query("archivePath"), offset, limit)
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
	api.GET("/task-reports/:id", func(c *gin.Context) {
		report, file, err := store.TaskReportFile(c.Param("id"))
		if err != nil {
			writeError(c, http.StatusNotFound, err)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || info.Size() != report.Size {
			writeError(c, http.StatusNotFound, errors.New("任务报告文件不存在或不完整"))
			return
		}
		disposition := mime.FormatMediaType("attachment", map[string]string{"filename": report.Name})
		c.DataFromReader(http.StatusOK, info.Size(), report.MimeType, file, map[string]string{
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
			Body      string `json:"body"`
			Objective string `json:"objective"`
		}
		if !bindJSON(c, &in) {
			return
		}
		v, err := manager.AddIssueCommentWithObjective(c.Param("id"), in.Body, in.Objective)
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
	api.POST("/settings/web-search/test", func(c *gin.Context) {
		var in control.WebSearchTestInput
		if !bindJSON(c, &in) {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()
		result, err := store.TestWebSearch(ctx, in)
		if err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	api.PUT("/settings/web-search", func(c *gin.Context) {
		var in control.WebSearchConfig
		if !bindJSON(c, &in) {
			return
		}
		if _, err := store.SaveWebSearchConfig(in); err != nil {
			writeError(c, http.StatusUnprocessableEntity, err)
			return
		}
		c.JSON(http.StatusOK, store.State())
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
		c.JSON(200, store.State())
	}
	api.POST("/setup/complete", saveConfig)
	api.PUT("/settings", saveConfig)
	frontend := http.FileSystem(http.FS(webui.Files()))
	if info, err := os.Stat(filepath.Join(dist, "index.html")); err == nil && !info.IsDir() {
		frontend = http.Dir(dist)
	}
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			writeError(c, 404, errors.New("endpoint not found"))
			return
		}
		requested := filepath.ToSlash(filepath.Clean(c.Request.URL.Path))
		if serveFrontendFile(c, frontend, requested) {
			return
		}
		if serveFrontendFile(c, frontend, "/index.html") {
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "frontend assets are unavailable"})
	})
	return r
}

func serveFrontendFile(c *gin.Context, frontend http.FileSystem, name string) bool {
	file, err := frontend.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return false
	}
	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), file)
	return true
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

func receiveInputAttachment(c *gin.Context, store *control.Store, scope, ownerID string) {
	const multipartOverhead = int64(8 << 20)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, control.MaxInputAttachmentSize+multipartOverhead)
	reader, err := c.Request.MultipartReader()
	if err != nil {
		writeError(c, http.StatusBadRequest, errors.New("附件请求必须使用 multipart/form-data"))
		return
	}
	for {
		part, partErr := reader.NextPart()
		if errors.Is(partErr, io.EOF) {
			break
		}
		if partErr != nil {
			writeError(c, http.StatusBadRequest, errors.New("读取附件上传流失败或请求超过大小限制"))
			return
		}
		if part.FormName() != "file" || strings.TrimSpace(part.FileName()) == "" {
			_ = part.Close()
			continue
		}
		attachment, uploadErr := store.StageInputAttachment(scope, ownerID, part.FileName(), part.Header.Get("Content-Type"), part)
		_ = part.Close()
		if uploadErr != nil {
			writeError(c, http.StatusUnprocessableEntity, uploadErr)
			return
		}
		c.JSON(http.StatusCreated, attachment)
		return
	}
	writeError(c, http.StatusBadRequest, errors.New("请选择要上传的附件"))
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

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
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

func parseBoolQuery(value string, defaultValue bool) (bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, errors.New("invalid boolean")
	}
	return parsed, nil
}

func archiveFilenamePart(value string) string {
	value = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	return value
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

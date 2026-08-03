package control

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

var containerImagePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]{0,254}$`)
var ErrContainerProfileReferenced = errors.New("容器执行环境已被引用")

const WorkerContainerImage = "aegis-worker:latest"

// TaskWorkspacePath is the only model-visible filesystem root. Every Task owns
// one Docker volume mounted here; Issues and executions never receive a host
// path or a private workspace root of their own.
const TaskWorkspacePath = "/workspace"

func (s *Store) ContainerProfiles() []ContainerProfile {
	var profiles []ContainerProfile
	s.db.Order("created_at asc").Find(&profiles)
	return profiles
}

func (s *Store) GetContainerProfile(id string) (ContainerProfile, error) {
	var profile ContainerProfile
	if err := s.db.First(&profile, "id = ?", id).Error; err != nil {
		return ContainerProfile{}, errors.New("container profile not found")
	}
	return profile, nil
}

func (s *Store) DefaultContainerProfile() (ContainerProfile, error) {
	var profile ContainerProfile
	err := s.db.Where("enabled = ?", true).
		Order("case when lower(name) = 'default' then 0 else 1 end").
		Order("created_at asc").First(&profile).Error
	if err != nil {
		return ContainerProfile{}, errors.New("没有可用的 Docker 容器配置，请先在容器管理中创建并启用一个配置")
	}
	return profile, nil
}

func (s *Store) SaveContainerProfile(id string, input SaveContainerProfileInput) (ContainerProfile, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Image = WorkerContainerImage
	input.WorkspacePath = TaskWorkspacePath
	input.NetworkMode = fallback(strings.TrimSpace(input.NetworkMode), "bridge")
	if input.Name == "" || !containerImagePattern.MatchString(input.Image) {
		return ContainerProfile{}, errors.New("容器名称和有效的 Docker Image 必填")
	}
	if !slices.Contains([]string{"bridge", "none"}, input.NetworkMode) {
		return ContainerProfile{}, errors.New("当前仅支持 bridge 或 none 网络模式")
	}
	if input.MemoryMB < 0 || input.MemoryMB > 262144 || input.CPUs < 0 || input.CPUs > 128 {
		return ContainerProfile{}, errors.New("容器资源限制无效")
	}
	now := time.Now()
	profile := ContainerProfile{ID: id, Name: input.Name, Description: input.Description, Image: input.Image, WorkspacePath: input.WorkspacePath, NetworkMode: input.NetworkMode, MemoryMB: input.MemoryMB, CPUs: input.CPUs, Enabled: input.Enabled, CreatedAt: now, UpdatedAt: now}
	if id == "" {
		profile.ID = nextID("container-profile")
	} else {
		var existing ContainerProfile
		if err := s.db.First(&existing, "id = ?", id).Error; err != nil {
			return ContainerProfile{}, errors.New("container profile not found")
		}
		profile.CreatedAt = existing.CreatedAt
	}
	if err := s.db.Save(&profile).Error; err != nil {
		return ContainerProfile{}, err
	}
	s.notify()
	return profile, nil
}

func taskContainerName(id string) string { return "aegis-task-" + id }

func taskContainerVolumeName(id string) string { return "aegis-task-data-" + id }

func containerRuntimeStatus(name string) string {
	if _, err := exec.LookPath("docker"); err != nil {
		return "unavailable"
	}
	output, err := exec.Command("docker", "inspect", "--format", "{{.State.Status}}", name).CombinedOutput()
	if err != nil {
		if strings.Contains(strings.ToLower(string(output)), "no such") {
			return "missing"
		}
		return "unavailable"
	}
	status := strings.TrimSpace(string(output))
	if status == "" {
		return "unavailable"
	}
	return status
}

func containerRuntimeState(container ContainerInstance) ContainerInstance {
	container.RuntimeStatus = containerRuntimeStatus(container.Name)
	return container
}

func (s *Store) Containers() []ContainerInstance {
	var containers []ContainerInstance
	s.db.Order("created_at desc").Find(&containers)
	for index := range containers {
		containers[index] = containerRuntimeState(containers[index])
	}
	return containers
}

func (s *Store) GetContainer(id string) (ContainerInstance, error) {
	var container ContainerInstance
	if err := s.db.First(&container, "id = ?", id).Error; err != nil {
		return ContainerInstance{}, errors.New("container not found")
	}
	return containerRuntimeState(container), nil
}

// createTaskContainerBinding snapshots one enabled profile into the Task's
// durable, one-to-one container record. It intentionally does not start
// Docker; task publication owns allocation, while execution owns runtime
// startup.
func (s *Store) createTaskContainerBinding(task Task) (ContainerInstance, error) {
	if strings.TrimSpace(task.ContainerProfileID) == "" {
		return ContainerInstance{}, errors.New("任务未配置容器环境")
	}
	var existing ContainerInstance
	if err := s.db.First(&existing, "task_id = ?", task.ID).Error; err == nil {
		if existing.ContainerProfileID != task.ContainerProfileID {
			return ContainerInstance{}, errors.New("任务绑定的容器与当前环境配置不一致")
		}
		if err = s.bindTaskContainer(task.ID, existing.ID); err != nil {
			return ContainerInstance{}, err
		}
		existing.RuntimeStatus = "missing"
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return ContainerInstance{}, err
	}
	profile, err := s.GetContainerProfile(task.ContainerProfileID)
	if err != nil || !profile.Enabled {
		return ContainerInstance{}, errors.New("容器执行环境不存在或已停用")
	}
	now := time.Now()
	container := ContainerInstance{
		ID: nextID("container"), ContainerProfileID: profile.ID, TaskID: task.ID,
		Image:         profile.Image,
		WorkspacePath: profile.WorkspacePath, NetworkMode: profile.NetworkMode,
		MemoryMB: profile.MemoryMB, CPUs: profile.CPUs, CreatedAt: now, UpdatedAt: now,
	}
	container.Name = taskContainerName(container.ID)
	if err = s.db.Create(&container).Error; err != nil {
		return ContainerInstance{}, err
	}
	if err = s.bindTaskContainer(task.ID, container.ID); err != nil {
		_ = s.db.Delete(&ContainerInstance{}, "id = ?", container.ID).Error
		return ContainerInstance{}, err
	}
	container.RuntimeStatus = "missing"
	return container, nil
}

func (s *Store) ensureTaskContainer(issue Issue) (ContainerInstance, error) {
	if strings.TrimSpace(issue.ContainerProfileID) == "" {
		return ContainerInstance{}, errors.New("Issue 未配置容器环境")
	}
	taskID, err := s.taskIDForIssue(issue)
	standaloneIssue := false
	containerOwner := issue
	if err != nil {
		containerOwner, err = s.rootIssue(issue)
		if err != nil {
			return ContainerInstance{}, err
		}
		standaloneIssue = true
		if containerOwner.Hidden && strings.HasPrefix(containerOwner.ID, "employee-home-") {
			taskID = "employee-session-" + containerOwner.AssigneeAgentID
		} else {
			taskID = "issue-tree-" + containerOwner.ID
		}
	}
	var task Task
	if !standaloneIssue {
		err = s.db.First(&task, "id = ?", taskID).Error
	}
	if err != nil {
		return ContainerInstance{}, errors.New("task not found")
	}
	var container ContainerInstance
	containerID := task.ContainerID
	if standaloneIssue {
		containerID = containerOwner.ContainerID
	}
	if containerID != "" {
		container, err = s.GetContainer(containerID)
		if err != nil {
			containerID = ""
		}
	}
	if containerID == "" {
		err = s.db.First(&container, "task_id = ?", taskID).Error
	}
	if containerID == "" {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			profile, profileErr := s.GetContainerProfile(issue.ContainerProfileID)
			if profileErr != nil || !profile.Enabled {
				return ContainerInstance{}, errors.New("容器执行环境不存在或已停用")
			}
			now := time.Now()
			container = ContainerInstance{
				ID: nextID("container"), ContainerProfileID: profile.ID, TaskID: taskID,
				Image:         profile.Image,
				WorkspacePath: profile.WorkspacePath, NetworkMode: profile.NetworkMode,
				MemoryMB: profile.MemoryMB, CPUs: profile.CPUs, CreatedAt: now, UpdatedAt: now,
			}
			container.Name = taskContainerName(container.ID)
			if err = s.db.Create(&container).Error; err != nil {
				return ContainerInstance{}, err
			}
		} else if err != nil {
			return ContainerInstance{}, err
		}
	}
	if container.ContainerProfileID != issue.ContainerProfileID {
		return ContainerInstance{}, errors.New("任务绑定的容器与当前环境配置不一致")
	}
	if standaloneIssue {
		if err = s.db.Model(&Issue{}).Where("id IN ?", uniqueStrings([]string{containerOwner.ID, issue.ID})).Updates(map[string]any{"container_id": container.ID, "updated_at": time.Now()}).Error; err != nil {
			return ContainerInstance{}, err
		}
	} else if err = s.bindTaskContainer(task.ID, container.ID); err != nil {
		return ContainerInstance{}, err
	}
	container, err = s.StartContainer(container.ID)
	if err != nil {
		return ContainerInstance{}, err
	}
	s.notify()
	return container, nil
}

func (s *Store) rootIssue(issue Issue) (Issue, error) {
	current := issue
	for strings.TrimSpace(current.ParentID) != "" {
		var parent Issue
		if err := s.db.First(&parent, "id = ?", current.ParentID).Error; err != nil {
			return Issue{}, errors.New("Issue 根节点不存在")
		}
		current = parent
	}
	return current, nil
}

func (s *Store) taskIDForIssue(issue Issue) (string, error) {
	current := issue
	for {
		if current.TaskSourceID != "" {
			return current.TaskSourceID, nil
		}
		if current.ParentID == "" {
			return "", errors.New("Issue 未关联任务定义")
		}
		var parent Issue
		if err := s.db.First(&parent, "id = ?", current.ParentID).Error; err != nil {
			return "", errors.New("Issue 根任务不存在")
		}
		current = parent
	}
}

// ensureTaskVolume allocates the Task workspace without starting the Task
// container. Docker run also performs this operation defensively, but explicit
// allocation at task publication makes the workspace lifecycle Task-owned.
func (s *Store) ensureTaskVolume(container ContainerInstance) error {
	if err := ProbeDocker(); err != nil {
		return err
	}
	name := taskContainerVolumeName(container.ID)
	if err := exec.Command("docker", "volume", "inspect", name).Run(); err == nil {
		return nil
	}
	output, err := exec.Command(
		"docker", "volume", "create",
		"--label", "aegis.managed=true",
		"--label", "aegis.container-id="+container.ID,
		"--label", "aegis.task-id="+container.TaskID,
		name,
	).CombinedOutput()
	if err != nil {
		return fmt.Errorf("创建任务 Workspace 失败: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func (s *Store) bindTaskContainer(taskID, containerID string) error {
	issueIDs, err := s.issueIDsForTask(taskID)
	if err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Task{}).Where("id = ?", taskID).Updates(map[string]any{"container_id": containerID, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		if len(issueIDs) > 0 {
			return tx.Model(&Issue{}).Where("id IN ?", issueIDs).Updates(map[string]any{"container_id": containerID, "updated_at": time.Now()}).Error
		}
		return nil
	})
}

func (s *Store) issueIDsForTask(taskID string) ([]string, error) {
	var issues []Issue
	if err := s.db.Select("id", "parent_id", "task_source_id").Find(&issues).Error; err != nil {
		return nil, err
	}
	set := make(map[string]bool)
	for _, issue := range issues {
		if issue.TaskSourceID == taskID {
			set[issue.ID] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, issue := range issues {
			if issue.ParentID != "" && set[issue.ParentID] && !set[issue.ID] {
				set[issue.ID] = true
				changed = true
			}
		}
	}
	ids := make([]string, 0, len(set))
	for _, issue := range issues {
		if set[issue.ID] {
			ids = append(ids, issue.ID)
		}
	}
	return ids, nil
}

func (s *Store) StartContainer(id string) (ContainerInstance, error) {
	container, err := s.GetContainer(id)
	if err != nil {
		return ContainerInstance{}, err
	}
	if err = ProbeDocker(); err != nil {
		return ContainerInstance{}, err
	}
	if err = s.ensureTaskVolume(container); err != nil {
		return ContainerInstance{}, err
	}
	container = containerRuntimeState(container)
	switch container.RuntimeStatus {
	case "running":
		return container, nil
	case "created", "exited", "stopped":
		if output, startErr := exec.Command("docker", "start", container.Name).CombinedOutput(); startErr != nil {
			return ContainerInstance{}, fmt.Errorf("启动容器失败: %s", strings.TrimSpace(string(output)))
		}
	case "paused":
		if output, unpauseErr := exec.Command("docker", "unpause", container.Name).CombinedOutput(); unpauseErr != nil {
			return ContainerInstance{}, fmt.Errorf("恢复容器失败: %s", strings.TrimSpace(string(output)))
		}
	case "restarting", "removing":
		return ContainerInstance{}, fmt.Errorf("容器当前状态为 %s，请稍后重试", container.RuntimeStatus)
	case "dead":
		if output, removeErr := exec.Command("docker", "rm", "-f", container.Name).CombinedOutput(); removeErr != nil {
			return ContainerInstance{}, fmt.Errorf("清理异常容器失败: %s", strings.TrimSpace(string(output)))
		}
		if err = s.createContainerRuntime(container); err != nil {
			return ContainerInstance{}, err
		}
	default:
		if err = s.createContainerRuntime(container); err != nil {
			return ContainerInstance{}, err
		}
	}
	_ = s.db.Model(&ContainerInstance{}).Where("id = ?", id).Update("updated_at", time.Now()).Error
	s.notify()
	return s.GetContainer(id)
}

func (s *Store) createContainerRuntime(container ContainerInstance) error {
	args := []string{
		"run", "--detach", "--name", container.Name,
		"--label", "aegis.managed=true",
		"--label", "aegis.container-id=" + container.ID,
		"--label", "aegis.profile-id=" + container.ContainerProfileID,
		"--label", "aegis.task-id=" + container.TaskID,
		"--workdir", container.WorkspacePath, "--network", container.NetworkMode,
		"--add-host", "host.docker.internal:host-gateway",
	}
	if container.MemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", container.MemoryMB))
	}
	if container.CPUs > 0 {
		args = append(args, "--cpus", strconv.FormatFloat(container.CPUs, 'f', -1, 64))
	}
	args = append(args,
		"--volume", taskContainerVolumeName(container.ID)+":"+container.WorkspacePath,
	)
	args = append(args, container.Image, "sleep", "infinity")
	if output, runErr := exec.Command("docker", args...).CombinedOutput(); runErr != nil {
		return fmt.Errorf("创建容器失败: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func (s *Store) StopContainer(id string) (ContainerInstance, error) {
	container, err := s.GetContainer(id)
	if err != nil {
		return ContainerInstance{}, err
	}
	if container.RuntimeStatus == "unavailable" {
		if err = ProbeDocker(); err != nil {
			return ContainerInstance{}, err
		}
		container = containerRuntimeState(container)
	}
	if !slices.Contains([]string{"running", "paused", "restarting"}, container.RuntimeStatus) {
		return container, nil
	}
	if output, stopErr := exec.Command("docker", "stop", "--time", "5", container.Name).CombinedOutput(); stopErr != nil {
		return ContainerInstance{}, fmt.Errorf("停止容器失败: %s", strings.TrimSpace(string(output)))
	}
	_ = s.db.Model(&ContainerInstance{}).Where("id = ?", id).Update("updated_at", time.Now()).Error
	s.notify()
	return s.GetContainer(id)
}

// reconcileFinishedTaskContainer stops the task runtime only after the root
// has ended (or entered human review), every descendant has ended, and no
// Agent Execution can still be using /workspace. The container record and its
// named volume are intentionally retained so continuation can restart in place.
func (m *Manager) reconcileFinishedTaskContainer(issue Issue) {
	if m == nil || m.store == nil {
		return
	}
	if err := m.stopFinishedTaskContainer(issue); err != nil {
		root, rootErr := m.store.taskRoot(issue)
		if rootErr == nil {
			m.store.addEvent(root.CurrentExecutionID, root.ID, "container", "任务结束后自动停止容器失败", err.Error())
			m.store.notify()
		}
	}
}

func (m *Manager) stopFinishedTaskContainer(issue Issue) error {
	root, err := m.store.taskRoot(issue)
	if err != nil {
		return err
	}
	if root.Hidden || root.TaskSourceID == "" || root.ContainerID == "" || (!issueStatusTerminal(root.Status) && root.Status != "in_review") {
		return nil
	}
	m.containerMu.Lock()
	defer m.containerMu.Unlock()

	var candidates []Issue
	if err = m.store.db.Where("project_id = ?", root.ProjectID).Find(&candidates).Error; err != nil {
		return err
	}
	children := make(map[string][]Issue)
	for _, candidate := range candidates {
		children[candidate.ParentID] = append(children[candidate.ParentID], candidate)
	}
	issueIDs := make([]string, 0, len(candidates))
	queue, seen := []string{root.ID}, map[string]bool{}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		issueIDs = append(issueIDs, id)
		for _, child := range children[id] {
			if !issueStatusTerminal(child.Status) {
				return nil
			}
			queue = append(queue, child.ID)
		}
	}
	var active int64
	if err = m.store.db.Model(&Execution{}).Where("issue_id IN ? AND status IN ?", issueIDs, activeExecutionStatuses).Count(&active).Error; err != nil {
		return err
	}
	if active > 0 {
		return nil
	}
	container, err := m.store.GetContainer(root.ContainerID)
	if err != nil {
		return err
	}
	if !slices.Contains([]string{"running", "paused", "restarting"}, container.RuntimeStatus) {
		return nil
	}
	stopped, err := m.store.StopContainer(container.ID)
	if err != nil {
		return err
	}
	m.store.addEvent(root.CurrentExecutionID, root.ID, "container", "任务结束，容器已自动停止", fmt.Sprintf("容器 %s 已停止；任务 named volume 与 /workspace 数据保留，可在继续任务时原地恢复。", stopped.Name))
	return nil
}

func removeContainerRuntime(container ContainerInstance) error {
	if err := ProbeDocker(); err != nil {
		return err
	}
	if output, err := exec.Command("docker", "rm", "-f", container.Name).CombinedOutput(); err != nil && !strings.Contains(strings.ToLower(string(output)), "no such") {
		return fmt.Errorf("删除容器失败: %s", strings.TrimSpace(string(output)))
	}
	volume := taskContainerVolumeName(container.ID)
	if output, err := exec.Command("docker", "volume", "rm", "-f", volume).CombinedOutput(); err != nil && !strings.Contains(strings.ToLower(string(output)), "no such") {
		return fmt.Errorf("删除容器工作区失败: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func BuildWorkerContainerImage(ctx context.Context) (string, error) {
	if err := ProbeDocker(); err != nil {
		return "", err
	}
	root, err := dockerBuildContext()
	if err != nil {
		return "", err
	}
	command := exec.CommandContext(ctx, "docker", "build", "--tag", WorkerContainerImage, root)
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("构建 Worker 镜像失败: %s", strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func dockerBuildContext() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if info, statErr := os.Stat(filepath.Join(dir, "Dockerfile")); statErr == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("找不到 Aegis 项目根目录中的 Dockerfile")
		}
		dir = parent
	}
}

type containerProfileDeletePlan struct {
	Impact          ContainerProfileDeleteImpact
	IssueIDs        []string
	TaskIDs         []string
	ExecutionIDs    []string
	AttachmentPaths []string
}

func (s *Store) containerProfileDeletePlan(id string) (containerProfileDeletePlan, error) {
	var profile ContainerProfile
	if err := s.db.First(&profile, "id = ?", id).Error; err != nil {
		return containerProfileDeletePlan{}, errors.New("container profile not found")
	}

	var tasks []Task
	if err := s.db.Select("id").Where("container_profile_id = ?", id).Find(&tasks).Error; err != nil {
		return containerProfileDeletePlan{}, err
	}
	var containerCount int64
	if err := s.db.Model(&ContainerInstance{}).Where("container_profile_id = ?", id).Count(&containerCount).Error; err != nil {
		return containerProfileDeletePlan{}, err
	}
	taskIDs := make([]string, 0, len(tasks))
	taskSet := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
		taskSet[task.ID] = true
	}

	var issues []Issue
	if err := s.db.Select("id", "parent_id", "task_source_id", "container_profile_id").Find(&issues).Error; err != nil {
		return containerProfileDeletePlan{}, err
	}
	issueSet := make(map[string]bool)
	for _, issue := range issues {
		if issue.ContainerProfileID == id || taskSet[issue.TaskSourceID] {
			issueSet[issue.ID] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, issue := range issues {
			if issue.ParentID != "" && issueSet[issue.ParentID] && !issueSet[issue.ID] {
				issueSet[issue.ID] = true
				changed = true
			}
		}
	}
	issueIDs := make([]string, 0, len(issueSet))
	for _, issue := range issues {
		if issueSet[issue.ID] {
			issueIDs = append(issueIDs, issue.ID)
		}
	}

	var executions []Execution
	if len(issueIDs) > 0 {
		if err := s.db.Select("id", "status").Where("issue_id IN ?", issueIDs).Find(&executions).Error; err != nil {
			return containerProfileDeletePlan{}, err
		}
	}
	executionIDs := make([]string, 0, len(executions))
	activeExecutions := 0
	for _, execution := range executions {
		executionIDs = append(executionIDs, execution.ID)
		if slices.Contains(activeExecutionStatuses, execution.Status) {
			activeExecutions++
		}
	}
	var attachments []IssueAttachment
	if len(issueIDs) > 0 {
		if err := s.db.Select("storage_path").Where("issue_id IN ?", issueIDs).Find(&attachments).Error; err != nil {
			return containerProfileDeletePlan{}, err
		}
	}
	attachmentPaths := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if strings.TrimSpace(attachment.StoragePath) != "" {
			attachmentPaths = append(attachmentPaths, attachment.StoragePath)
		}
	}
	return containerProfileDeletePlan{
		Impact: ContainerProfileDeleteImpact{
			ContainerProfileID: id, ContainerCount: int(containerCount), IssueCount: len(issueIDs), TaskCount: len(taskIDs),
			ExecutionCount: len(executionIDs), ActiveExecutionCount: activeExecutions,
		},
		IssueIDs: issueIDs, TaskIDs: taskIDs, ExecutionIDs: executionIDs, AttachmentPaths: attachmentPaths,
	}, nil
}

func (s *Store) ContainerProfileDeleteImpact(id string) (ContainerProfileDeleteImpact, error) {
	plan, err := s.containerProfileDeletePlan(id)
	return plan.Impact, err
}

func (s *Store) DeleteContainerProfile(id string, cascadeIssues bool) (ContainerProfileDeleteResult, error) {
	plan, err := s.containerProfileDeletePlan(id)
	if err != nil {
		return ContainerProfileDeleteResult{}, err
	}
	if plan.Impact.ContainerCount > 0 || plan.Impact.IssueCount > 0 || plan.Impact.TaskCount > 0 {
		return ContainerProfileDeleteResult{}, fmt.Errorf(
			"%w：关联 %d 个容器、%d 个 Issues 和 %d 个任务；请先删除关联容器和任务",
			ErrContainerProfileReferenced, plan.Impact.ContainerCount, plan.Impact.IssueCount, plan.Impact.TaskCount,
		)
	}
	result := ContainerProfileDeleteResult{ContainerProfileID: id}
	deleted := s.db.Delete(&ContainerProfile{}, "id = ?", id)
	err = deleted.Error
	if err == nil && deleted.RowsAffected == 0 {
		err = errors.New("container profile not found")
	}
	if err != nil {
		return ContainerProfileDeleteResult{}, err
	}
	s.notify()
	return result, nil
}

type containerDeletePlan struct {
	Container       ContainerInstance
	Impact          ContainerDeleteImpact
	IssueIDs        []string
	ExecutionIDs    []string
	AttachmentPaths []string
}

func (s *Store) containerDeletePlan(id string) (containerDeletePlan, error) {
	container, err := s.GetContainer(id)
	if err != nil {
		return containerDeletePlan{}, err
	}
	issueIDs, err := s.issueIDsForTask(container.TaskID)
	if err != nil {
		return containerDeletePlan{}, err
	}
	var executions []Execution
	if len(issueIDs) > 0 {
		if err = s.db.Select("id", "status").Where("issue_id IN ?", issueIDs).Find(&executions).Error; err != nil {
			return containerDeletePlan{}, err
		}
	}
	executionIDs := make([]string, 0, len(executions))
	activeExecutions := 0
	for _, execution := range executions {
		executionIDs = append(executionIDs, execution.ID)
		if slices.Contains(activeExecutionStatuses, execution.Status) {
			activeExecutions++
		}
	}
	var attachments []IssueAttachment
	if len(issueIDs) > 0 {
		if err = s.db.Select("storage_path").Where("issue_id IN ?", issueIDs).Find(&attachments).Error; err != nil {
			return containerDeletePlan{}, err
		}
	}
	attachmentPaths := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		if strings.TrimSpace(attachment.StoragePath) != "" {
			attachmentPaths = append(attachmentPaths, attachment.StoragePath)
		}
	}
	return containerDeletePlan{
		Container: container,
		Impact: ContainerDeleteImpact{
			ContainerID: container.ID, TaskID: container.TaskID, IssueCount: len(issueIDs),
			ExecutionCount: len(executionIDs), ActiveExecutionCount: activeExecutions,
		},
		IssueIDs: issueIDs, ExecutionIDs: executionIDs, AttachmentPaths: attachmentPaths,
	}, nil
}

func (s *Store) ContainerDeleteImpact(id string) (ContainerDeleteImpact, error) {
	plan, err := s.containerDeletePlan(id)
	return plan.Impact, err
}

func normalizeContainerBatchIDs(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, errors.New("至少选择一个容器")
	}
	if len(ids) > 500 {
		return nil, errors.New("一次最多操作 500 个容器")
	}
	seen := make(map[string]bool, len(ids))
	normalized := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, errors.New("容器 ID 不能为空")
		}
		if !seen[id] {
			seen[id] = true
			normalized = append(normalized, id)
		}
	}
	return normalized, nil
}

func (s *Store) StopContainers(ids []string) (ContainerBatchStopResult, error) {
	ids, err := normalizeContainerBatchIDs(ids)
	if err != nil {
		return ContainerBatchStopResult{}, err
	}
	result := ContainerBatchStopResult{
		Requested: len(ids),
		Stopped:   make([]ContainerInstance, 0, len(ids)),
		Failed:    make([]ContainerBatchFailure, 0),
	}
	for _, id := range ids {
		container, stopErr := s.StopContainer(id)
		if stopErr != nil {
			result.Failed = append(result.Failed, ContainerBatchFailure{ContainerID: id, Error: stopErr.Error()})
			continue
		}
		result.Stopped = append(result.Stopped, container)
	}
	return result, nil
}

func (s *Store) ContainerBatchDeleteImpact(ids []string) (ContainerBatchDeleteImpact, error) {
	ids, err := normalizeContainerBatchIDs(ids)
	if err != nil {
		return ContainerBatchDeleteImpact{}, err
	}
	result := ContainerBatchDeleteImpact{
		ContainerIDs:   append([]string(nil), ids...),
		ContainerCount: len(ids),
		Items:          make([]ContainerDeleteImpact, 0, len(ids)),
	}
	tasks := make(map[string]bool, len(ids))
	for _, id := range ids {
		impact, impactErr := s.ContainerDeleteImpact(id)
		if impactErr != nil {
			return ContainerBatchDeleteImpact{}, impactErr
		}
		result.Items = append(result.Items, impact)
		if impact.TaskID != "" {
			tasks[impact.TaskID] = true
		}
		result.IssueCount += impact.IssueCount
		result.ExecutionCount += impact.ExecutionCount
		result.ActiveExecutionCount += impact.ActiveExecutionCount
	}
	result.TaskCount = len(tasks)
	return result, nil
}

func (s *Store) DeleteContainer(id string, cascadeIssues bool) (ContainerDeleteResult, error) {
	plan, err := s.containerDeletePlan(id)
	if err != nil {
		return ContainerDeleteResult{}, err
	}
	if (plan.Impact.IssueCount > 0 || plan.Impact.ExecutionCount > 0) && !cascadeIssues {
		return ContainerDeleteResult{}, fmt.Errorf(
			"%w：关联 1 个任务、%d 个 Issues 和 %d 条执行记录；确认级联删除后重试",
			ErrContainerProfileReferenced, plan.Impact.IssueCount, plan.Impact.ExecutionCount,
		)
	}
	if err = removeContainerRuntime(plan.Container); err != nil {
		return ContainerDeleteResult{}, err
	}
	result := ContainerDeleteResult{ContainerID: id}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if len(plan.IssueIDs) > 0 {
			if err := tx.Where("issue_id IN ? OR related_issue_id IN ?", plan.IssueIDs, plan.IssueIDs).Delete(&IssueRelation{}).Error; err != nil {
				return err
			}
			for _, model := range []any{&ConciergeConversation{}, &IssueValidation{}, &ExecutionEvent{}, &ExecutionProgress{}, &Message{}, &Approval{}, &IssueComment{}, &IssueAttachment{}, &AgentWakeup{}} {
				if err := tx.Where("issue_id IN ?", plan.IssueIDs).Delete(model).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("parent_issue_id IN ?", plan.IssueIDs).Delete(&IssueDecomposition{}).Error; err != nil {
				return err
			}
			if err := tx.Where("parent_issue_id IN ?", plan.IssueIDs).Delete(&IssueChildWait{}).Error; err != nil {
				return err
			}
		}
		if len(plan.ExecutionIDs) > 0 {
			for _, model := range []any{&ExecutionEvent{}, &ExecutionProgress{}, &Message{}, &Approval{}, &IssueComment{}, &IssueAttachment{}} {
				if err := tx.Where("execution_id IN ?", plan.ExecutionIDs).Delete(model).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("source_execution_id IN ? OR validation_execution_id IN ?", plan.ExecutionIDs, plan.ExecutionIDs).Delete(&IssueValidation{}).Error; err != nil {
				return err
			}
			if err := tx.Where("source_execution_id IN ?", plan.ExecutionIDs).Delete(&IssueDecomposition{}).Error; err != nil {
				return err
			}
			if err := tx.Where("source_execution_id IN ?", plan.ExecutionIDs).Delete(&IssueChildWait{}).Error; err != nil {
				return err
			}
			executions := tx.Delete(&Execution{}, "id IN ?", plan.ExecutionIDs)
			if executions.Error != nil {
				return executions.Error
			}
			result.DeletedExecutions = executions.RowsAffected
		}
		if len(plan.IssueIDs) > 0 {
			issues := tx.Delete(&Issue{}, "id IN ?", plan.IssueIDs)
			if issues.Error != nil {
				return issues.Error
			}
			result.DeletedIssues = issues.RowsAffected
		}
		tasks := tx.Delete(&Task{}, "id = ?", plan.Container.TaskID)
		if tasks.Error != nil {
			return tasks.Error
		}
		result.DeletedTasks = tasks.RowsAffected
		deleted := tx.Delete(&ContainerInstance{}, "id = ?", id)
		if deleted.Error != nil {
			return deleted.Error
		}
		if deleted.RowsAffected == 0 {
			return errors.New("container not found")
		}
		return nil
	})
	if err != nil {
		return ContainerDeleteResult{}, err
	}
	for _, storagePath := range plan.AttachmentPaths {
		absolute := filepath.Join(s.dataDir, filepath.Clean(storagePath))
		if strings.HasPrefix(absolute, s.dataDir+string(os.PathSeparator)) {
			_ = os.RemoveAll(filepath.Dir(absolute))
		}
	}
	s.notify()
	return result, nil
}

func ProbeDocker() error {
	path, err := exec.LookPath("docker")
	if err != nil {
		return errors.New("未找到 Docker CLI")
	}
	if output, err := exec.Command(path, "version", "--format", "{{.Server.Version}}").CombinedOutput(); err != nil {
		return fmt.Errorf("Docker daemon 不可用: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

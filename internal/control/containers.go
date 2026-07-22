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

const WorkerContainerImage = "aegis-pi-worker:latest"

func (s *Store) ContainerProfiles() []ContainerProfile {
	var profiles []ContainerProfile
	s.db.Order("created_at asc").Find(&profiles)
	for index := range profiles {
		profiles[index] = containerRuntimeState(profiles[index])
	}
	return profiles
}

func (s *Store) GetContainerProfile(id string) (ContainerProfile, error) {
	var profile ContainerProfile
	if err := s.db.First(&profile, "id = ?", id).Error; err != nil {
		return ContainerProfile{}, errors.New("container profile not found")
	}
	return containerRuntimeState(profile), nil
}

func (s *Store) SaveContainerProfile(id string, input SaveContainerProfileInput) (ContainerProfile, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Image = WorkerContainerImage
	input.NodePath = fallback(strings.TrimSpace(input.NodePath), "node")
	input.PiPath = fallback(strings.TrimSpace(input.PiPath), "/usr/local/bin/pi")
	input.WorkspacePath = fallback(strings.TrimSpace(input.WorkspacePath), "/workspace")
	input.HostWorkspace = strings.TrimSpace(input.HostWorkspace)
	input.NetworkMode = fallback(strings.TrimSpace(input.NetworkMode), "bridge")
	if input.Name == "" || !containerImagePattern.MatchString(input.Image) {
		return ContainerProfile{}, errors.New("容器名称和有效的 Docker Image 必填")
	}
	if !strings.HasPrefix(input.WorkspacePath, "/") || strings.Contains(input.WorkspacePath, "..") {
		return ContainerProfile{}, errors.New("容器工作目录必须是未包含 .. 的绝对路径")
	}
	if input.HostWorkspace != "" && !filepath.IsAbs(input.HostWorkspace) {
		return ContainerProfile{}, errors.New("宿主机工作目录必须是绝对路径")
	}
	if !slices.Contains([]string{"bridge", "none"}, input.NetworkMode) {
		return ContainerProfile{}, errors.New("当前仅支持 bridge 或 none 网络模式")
	}
	if input.MemoryMB < 0 || input.MemoryMB > 262144 || input.CPUs < 0 || input.CPUs > 128 {
		return ContainerProfile{}, errors.New("容器资源限制无效")
	}
	now := time.Now()
	profile := ContainerProfile{ID: id, Name: input.Name, Description: input.Description, Image: input.Image, NodePath: input.NodePath, PiPath: input.PiPath, WorkspacePath: input.WorkspacePath, HostWorkspace: input.HostWorkspace, NetworkMode: input.NetworkMode, MemoryMB: input.MemoryMB, CPUs: input.CPUs, Enabled: input.Enabled, CreatedAt: now, UpdatedAt: now}
	if id == "" {
		profile.ID = nextID("container-profile")
	} else {
		var existing ContainerProfile
		if err := s.db.First(&existing, "id = ?", id).Error; err != nil {
			return ContainerProfile{}, errors.New("container profile not found")
		}
		if containerRuntimeState(existing).RuntimeStatus == "running" {
			return ContainerProfile{}, errors.New("请先停止容器再修改环境配置")
		}
		profile.CreatedAt = existing.CreatedAt
	}
	if err := s.db.Save(&profile).Error; err != nil {
		return ContainerProfile{}, err
	}
	s.notify()
	return profile, nil
}

func containerName(id string) string { return "aegis-env-" + id }

func containerRuntimeState(profile ContainerProfile) ContainerProfile {
	profile.ContainerName = containerName(profile.ID)
	profile.RuntimeStatus = "stopped"
	if output, err := exec.Command("docker", "inspect", "--format", "{{.State.Running}}", profile.ContainerName).Output(); err == nil && strings.TrimSpace(string(output)) == "true" {
		profile.RuntimeStatus = "running"
	}
	return profile
}

func (s *Store) StartContainerProfile(id string) (ContainerProfile, error) {
	profile, err := s.GetContainerProfile(id)
	if err != nil {
		return ContainerProfile{}, err
	}
	if !profile.Enabled {
		return ContainerProfile{}, errors.New("容器环境已停用")
	}
	if profile.HostWorkspace == "" {
		return ContainerProfile{}, errors.New("请先配置宿主机工作目录")
	}
	if info, statErr := os.Stat(profile.HostWorkspace); statErr != nil || !info.IsDir() {
		return ContainerProfile{}, errors.New("宿主机工作目录不存在")
	}
	if err = ProbeDocker(); err != nil {
		return ContainerProfile{}, err
	}
	name := containerName(profile.ID)
	_ = exec.Command("docker", "rm", "-f", name).Run()
	args := []string{"run", "--detach", "--name", name, "--workdir", profile.WorkspacePath, "--network", profile.NetworkMode, "--add-host", "host.docker.internal:host-gateway"}
	if profile.MemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", profile.MemoryMB))
	}
	if profile.CPUs > 0 {
		args = append(args, "--cpus", strconv.FormatFloat(profile.CPUs, 'f', -1, 64))
	}
	args = append(args, "--volume", profile.HostWorkspace+":"+profile.WorkspacePath, "--volume", filepath.Join(s.dataDir, "sessions")+":/aegis/sessions", "--volume", filepath.Join(s.dataDir, "runtime", "aegis-guard.ts")+":/aegis/runtime/aegis-guard.ts:ro")
	if info, statErr := os.Stat(filepath.Join(s.dataDir, "skills")); statErr == nil && info.IsDir() {
		args = append(args, "--volume", filepath.Join(s.dataDir, "skills")+":/aegis/skills:ro")
	}
	args = append(args, profile.Image, "sleep", "infinity")
	if output, runErr := exec.Command("docker", args...).CombinedOutput(); runErr != nil {
		return ContainerProfile{}, fmt.Errorf("启动容器失败: %s", strings.TrimSpace(string(output)))
	}
	s.notify()
	return s.GetContainerProfile(id)
}

func (s *Store) StopContainerProfile(id string) (ContainerProfile, error) {
	profile, err := s.GetContainerProfile(id)
	if err != nil {
		return ContainerProfile{}, err
	}
	name := containerName(profile.ID)
	if output, stopErr := exec.Command("docker", "stop", "--time", "5", name).CombinedOutput(); stopErr != nil && profile.RuntimeStatus == "running" {
		return ContainerProfile{}, fmt.Errorf("停止容器失败: %s", strings.TrimSpace(string(output)))
	}
	_ = exec.Command("docker", "rm", name).Run()
	s.notify()
	return s.GetContainerProfile(id)
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
			ContainerProfileID: id, IssueCount: len(issueIDs), TaskCount: len(taskIDs),
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
	if profile, err := s.GetContainerProfile(id); err == nil && profile.RuntimeStatus == "running" {
		return ContainerProfileDeleteResult{}, errors.New("请先停止容器再删除环境")
	}
	plan, err := s.containerProfileDeletePlan(id)
	if err != nil {
		return ContainerProfileDeleteResult{}, err
	}
	if (plan.Impact.IssueCount > 0 || plan.Impact.TaskCount > 0) && !cascadeIssues {
		return ContainerProfileDeleteResult{}, fmt.Errorf(
			"%w：关联 %d 个 Issues 和 %d 个任务定义；确认级联删除后重试",
			ErrContainerProfileReferenced, plan.Impact.IssueCount, plan.Impact.TaskCount,
		)
	}
	result := ContainerProfileDeleteResult{ContainerProfileID: id}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if len(plan.IssueIDs) > 0 {
			if err := tx.Where("issue_id IN ? OR related_issue_id IN ?", plan.IssueIDs, plan.IssueIDs).Delete(&IssueRelation{}).Error; err != nil {
				return err
			}
			for _, model := range []any{&ConciergeConversation{}, &IssueAgentSession{}, &IssueValidation{}, &ExecutionEvent{}, &ExecutionProgress{}, &Message{}, &Approval{}, &IssueComment{}, &IssueAttachment{}, &AgentWakeup{}} {
				if err := tx.Where("issue_id IN ?", plan.IssueIDs).Delete(model).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("source_issue_id IN ? OR task_id IN ?", plan.IssueIDs, plan.IssueIDs).Delete(&TaskBroadcast{}).Error; err != nil {
				return err
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
			if err := tx.Where("source_execution_id IN ?", plan.ExecutionIDs).Delete(&TaskBroadcast{}).Error; err != nil {
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
		if len(plan.TaskIDs) > 0 {
			tasks := tx.Delete(&Task{}, "id IN ?", plan.TaskIDs)
			if tasks.Error != nil {
				return tasks.Error
			}
			result.DeletedTasks = tasks.RowsAffected
		}
		deleted := tx.Delete(&ContainerProfile{}, "id = ?", id)
		if deleted.Error != nil {
			return deleted.Error
		}
		if deleted.RowsAffected == 0 {
			return errors.New("container profile not found")
		}
		return nil
	})
	if err != nil {
		return ContainerProfileDeleteResult{}, err
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

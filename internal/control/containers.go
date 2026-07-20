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
)

var containerImagePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]{0,254}$`)

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

func (s *Store) DeleteContainerProfile(id string) error {
	if profile, err := s.GetContainerProfile(id); err == nil && profile.RuntimeStatus == "running" {
		return errors.New("请先停止容器再删除环境")
	}
	var references int64
	s.db.Model(&Issue{}).Where("container_profile_id = ?", id).Count(&references)
	if references > 0 {
		return errors.New("容器执行环境已被 Issue 引用，不能删除；可以将其停用")
	}
	result := s.db.Delete(&ContainerProfile{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("container profile not found")
	}
	s.notify()
	return nil
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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"work2api/internal/selfupdate"
)

func updateDeployment(executable string, build selfupdate.Build) selfupdate.Deployment {
	d := selfupdate.Deployment{Kind: "binary"}
	if _, err := os.Stat("/.dockerenv"); err == nil || os.Getenv("WORK2API_CONTAINER") == "1" {
		return selfupdate.Deployment{Kind: "docker", Reason: "容器请更新镜像并重建；不在容器内替换程序"}
	}
	if !build.Release {
		return selfupdate.Deployment{Kind: "source", Reason: "源码自编译仅检查Release；请使用现有构建脚本更新"}
	}
	for dir, n := filepath.Dir(executable), 0; n < 4; n++ {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return selfupdate.Deployment{Kind: "source", Reason: "程序位于源码工作区；为保护本地改动，不覆盖安装Release"}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if os.Getenv("INVOCATION_ID") != "" || os.Getenv("SYSTEMD_EXEC_PID") != "" {
		d.Kind = "systemd"
		raw, err := os.ReadFile("/proc/self/cgroup")
		if err == nil {
			for _, segment := range strings.FieldsFunc(string(raw), func(c rune) bool { return c == '/' || c == '\n' }) {
				if regexp.MustCompile(`^[A-Za-z0-9_.@-]+\.service$`).MatchString(segment) {
					d.Unit = segment
				}
			}
		}
		if runtime.GOOS != "linux" || d.Unit == "" || strings.HasPrefix(d.Unit, "-") || os.Geteuid() != 0 {
			d.Reason = "systemd自动安装需要可验证的服务单元及外部更新权限；当前部署仅检查更新"
			return d
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		raw, err = exec.CommandContext(ctx, "systemctl", "show", d.Unit, "--property=MainPID", "--value").Output()
		if err != nil || strings.TrimSpace(string(raw)) != fmt.Sprint(os.Getpid()) {
			d.Reason = "无法验证systemd主进程归属"
			return d
		}
		if _, err = exec.LookPath("systemd-run"); err != nil {
			d.Reason = "缺少systemd-run外部更新助手支持"
			return d
		}
	}
	info, err := os.Lstat(executable)
	if err != nil || !info.Mode().IsRegular() {
		d.Reason = "运行程序不是可替换的普通文件"
		return d
	}
	probe, err := os.CreateTemp(filepath.Dir(executable), ".work2api-write-check-")
	if err != nil {
		d.Reason = "程序目录不可写，请由部署管理员更新"
		return d
	}
	probe.Close()
	os.Remove(probe.Name())
	d.CanInstall = true
	return d
}
func pendingStartup(data string, build selfupdate.Build) (selfupdate.Plan, bool) {
	var plan selfupdate.Plan
	if selfupdate.ReadJSON(filepath.Join(data, ".updates", "pending.json"), &plan) != nil {
		return plan, false
	}
	active := plan.Phase == "backup_ready" || plan.Phase == "verifying" || plan.Phase == "accepting" || plan.Phase == "acceptance_unknown" || plan.Phase == "recovery_required"
	executable, _ := os.Executable()
	executable, _ = filepath.EvalSymlinks(executable)
	if !active || plan.Target != executable || filepath.Dir(plan.Root) != filepath.Join(data, ".updates") {
		return plan, false
	}
	digest, err := selfupdate.HashFile(executable)
	// A changed binary stays closed even if its metadata is unexpected. The
	// helper must verify it or restore the old snapshot before public writes.
	return plan, err == nil && digest != plan.OldHash
}
func launchUpdateHelper(path string) error {
	var plan selfupdate.Plan
	if err := selfupdate.ReadJSON(path, &plan); err != nil {
		return err
	}
	helper := filepath.Join(plan.Root, "helper"+filepath.Ext(selfupdate.ExecutableName()))
	logFile, err := os.OpenFile(filepath.Join(plan.Root, "helper.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	var cmd *exec.Cmd
	if plan.Deployment.Kind == "systemd" {
		cmd = exec.Command("systemd-run", "--quiet", "--collect", "--unit=work2api-updater-"+strings.TrimPrefix(plan.ID, "transaction-"), "--property=Type=exec", helper, "-apply-update", path)
	} else {
		cmd = exec.Command(helper, "-apply-update", path)
		detachUpdate(cmd)
	}
	cmd.Dir = plan.CWD
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err = cmd.Start(); err != nil {
		return err
	}
	if plan.Deployment.Kind == "systemd" {
		return cmd.Wait()
	}
	go cmd.Wait()
	return nil
}
func privateControl(data, identity, method, transaction string) ([]byte, error) {
	var info instanceInfo
	raw, err := os.ReadFile(filepath.Join(data, ".instance.json"))
	if err != nil {
		return nil, err
	}
	if len(raw) > 4096 || json.Unmarshal(raw, &info) != nil || info.Identity != identity || len(info.Token) != 64 || !localURL(info.Control, "/instance") {
		return nil, errors.New("本地实例身份不匹配")
	}
	timeout := 30 * time.Second
	if method == "POST" {
		timeout = 4 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, info.Control, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+info.Token)
	req.Header.Set("X-Update-Transaction", transaction)
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("实例控制返回HTTP %d，旧服务或恢复副本已保留", response.StatusCode)
	}
	return body, nil
}
func saveUpdateResult(plan *selfupdate.Plan, phase, message string) {
	plan.Phase = phase
	plan.Message = message
	root := filepath.Dir(plan.Root)
	_ = selfupdate.WriteJSON(filepath.Join(plan.Root, "plan.json"), plan)
	_ = selfupdate.WriteJSON(filepath.Join(root, "pending.json"), plan)
	_ = selfupdate.WriteJSON(filepath.Join(root, "result.json"), map[string]string{"Phase": phase, "Message": message})
}
func waitOffline(plan selfupdate.Plan) (func(), error) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		release, err := lockInstance([]string{filepath.Join(plan.DataDir, ".instance.lock"), plan.DBPath + ".instance.lock"})
		if err == nil {
			return release, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return nil, errors.New("旧实例尚未退出或另一实例持有数据锁，未替换文件")
}
func validateUpdatePlan(path string, plan selfupdate.Plan) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	root, err := filepath.Abs(plan.Root)
	if err != nil {
		return err
	}
	if absolute != filepath.Join(root, "plan.json") || filepath.Base(filepath.Dir(root)) != ".updates" || !strings.HasPrefix(filepath.Base(root), "transaction-") || plan.ID != filepath.Base(root) || len(plan.Token) != 64 {
		return errors.New("更新事务路径无效")
	}
	for _, p := range []string{root, plan.Target, plan.Candidate.Path, plan.DataDir, plan.DBPath} {
		if !filepath.IsAbs(p) {
			return errors.New("更新路径必须为绝对路径")
		}
		if err = selfupdate.NoSymlinks(p); err != nil {
			return err
		}
	}
	if filepath.Dir(plan.Candidate.Path) == root || filepath.Dir(filepath.Dir(plan.Candidate.Path)) != filepath.Dir(root) || !strings.HasPrefix(filepath.Base(filepath.Dir(plan.Candidate.Path)), "candidate-") {
		return errors.New("候选路径不属于更新工作区")
	}
	current := currentBuild()
	if !current.Release || current.Repository != plan.Current.Repository || current.Version != plan.Current.Version || current.Commit != plan.Current.Commit {
		return errors.New("助手来源与原程序不匹配")
	}
	if plan.Deployment.Kind == "systemd" && !regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.@-]*\.service$`).MatchString(plan.Unit) {
		return errors.New("服务单元名无效")
	}
	if plan.Phase != "installing" {
		return errors.New("该事务不能重复执行；保留备份供恢复")
	}
	if hash, e := selfupdate.HashFile(plan.Target); e != nil || hash != plan.OldHash {
		return errors.New("原程序指纹不匹配")
	}
	if hash, e := selfupdate.HashFile(plan.Candidate.Path); e != nil || hash != plan.Candidate.BinaryHash {
		return errors.New("候选程序指纹不匹配")
	}
	return nil
}
func applyUpdate(path string) (resultErr error) {
	var plan selfupdate.Plan
	if err := selfupdate.ReadJSON(path, &plan); err != nil {
		return err
	}
	if err := validateUpdatePlan(path, plan); err != nil {
		return err
	}
	// Abort before touching the executable if draining or a coherent backup fails.
	if _, err := privateControl(plan.DataDir, plan.Identity, "POST", plan.Token); err != nil {
		saveUpdateResult(&plan, "failed", "更新前准备失败，旧服务继续运行")
		return err
	}
	if err := selfupdate.ReadJSON(path, &plan); err != nil {
		_, _ = privateControl(plan.DataDir, plan.Identity, "OPTIONS", plan.Token)
		return err
	}
	if plan.Phase != "backup_ready" || plan.BackupHash == "" {
		_, _ = privateControl(plan.DataDir, plan.Identity, "OPTIONS", plan.Token)
		return errors.New("缺少更新前完整备份")
	}
	if plan.Deployment.Kind == "systemd" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		stopErr := exec.CommandContext(ctx, "systemctl", "stop", plan.Unit).Run()
		cancel()
		if stopErr != nil {
			_, _ = privateControl(plan.DataDir, plan.Identity, "OPTIONS", plan.Token)
			saveUpdateResult(&plan, "failed", "systemd未确认停止，未替换程序")
			return stopErr
		}
	}
	release, err := waitOffline(plan)
	if err != nil {
		saveUpdateResult(&plan, "failed", err.Error())
		return err
	}
	old := filepath.Join(plan.Root, "previous"+filepath.Ext(selfupdate.ExecutableName()))
	if err = selfupdate.CopyFile(plan.Target, old, 0700); err != nil {
		release()
		saveUpdateResult(&plan, "failed", "保留原程序失败，尝试恢复旧服务")
		_ = startUpdated(plan)
		return err
	}
	mode, statErr := os.Stat(plan.Target)
	if statErr != nil {
		release()
		saveUpdateResult(&plan, "recovery_required", "原程序属性读取失败，备份保留")
		return statErr
	}
	if err = selfupdate.ReplaceFile(plan.Candidate.Path, plan.Target, mode.Mode().Perm()); err != nil {
		release()
		_ = startUpdated(plan)
		saveUpdateResult(&plan, "failed", "程序替换失败，尝试恢复旧服务")
		return err
	}
	plan.Phase = "verifying"
	if err = selfupdate.WriteJSON(filepath.Join(filepath.Dir(plan.Root), "pending.json"), plan); err != nil {
		_ = selfupdate.ReplaceFile(old, plan.Target, mode.Mode().Perm())
		release()
		_ = startUpdated(plan)
		return err
	}
	release()
	if err = startUpdated(plan); err == nil {
		err = verifyUpdated(plan)
	}
	if err == nil {
		saveUpdateResult(&plan, "complete", "更新成功，程序及数据恢复副本已保留")
		cleanupOldTransactions(plan)
		_ = os.RemoveAll(filepath.Dir(plan.Candidate.Path))
		return nil
	}
	if errors.Is(err, errAcceptanceUncertain) {
		saveUpdateResult(&plan, "acceptance_unknown", "新版本已验证，但开放结果未确认；不自动回滚，需管理员核实")
		return err
	}
	// New startup is gated: no post-upgrade user writes can be lost on rollback.
	stopCandidate(plan)
	release, lockErr := waitOffline(plan)
	if lockErr != nil {
		saveUpdateResult(&plan, "recovery_required", "新实例未确认停止，未恢复数据；请部署管理员处理")
		return errors.Join(err, lockErr)
	}
	if restoreErr := selfupdate.RestoreBackup(plan.Backup, plan.BackupHash, plan.Target, plan.DataDir, plan.DBPath); restoreErr != nil {
		release()
		saveUpdateResult(&plan, "recovery_required", "数据恢复失败，保留完整备份并停止自动操作")
		return errors.Join(err, restoreErr)
	}
	if replaceErr := selfupdate.ReplaceFile(old, plan.Target, mode.Mode().Perm()); replaceErr != nil {
		release()
		saveUpdateResult(&plan, "recovery_required", "旧程序恢复失败，保留完整备份")
		return errors.Join(err, replaceErr)
	}
	saveUpdateResult(&plan, "rolled_back", "新版本启动验证失败，已恢复更新前程序和数据")
	release()
	if restartErr := startUpdated(plan); restartErr != nil {
		saveUpdateResult(&plan, "recovery_required", "已恢复程序与数据，但旧服务需手动启动")
		return restartErr
	}
	return err
}
func startUpdated(plan selfupdate.Plan) error {
	if plan.Deployment.Kind == "systemd" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, "systemctl", "restart", plan.Unit).Run()
	}
	args := append([]string(nil), plan.Args...)
	args = append(args, "-data-dir", plan.DataDir, "-db", plan.DBPath, "-strict-port")
	cmd := exec.Command(plan.Target, args...)
	cmd.Dir = plan.CWD
	detachUpdate(cmd)
	logFile, err := os.OpenFile(filepath.Join(plan.Root, "service-start.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err = cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

var errAcceptanceUncertain = errors.New("更新开放结果未确认")

func verifyUpdated(plan selfupdate.Plan) error {
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := privateControl(plan.DataDir, plan.Identity, "PATCH", plan.Token)
		if err == nil {
			var build selfupdate.Build
			if json.Unmarshal(raw, &build) == nil && build.Release && build.Version == plan.Candidate.Build.Version && build.Commit == plan.Candidate.Build.Commit && build.Schema == plan.Candidate.Build.Schema && build.Repository == plan.Current.Repository {
				// Persist completion before opening public writes. Never rollback after accept.
				saveUpdateResult(&plan, "accepting", "新版本验证完成，等待开放确认")
				if _, err = privateControl(plan.DataDir, plan.Identity, "PUT", plan.Token); err != nil {
					return errAcceptanceUncertain
				}
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return errors.New("新版本就绪验证超时")
}
func stopCandidate(plan selfupdate.Plan) {
	// The candidate accepts this only while its startup gate is still closed.
	// A failure to confirm shutdown prevents any data restoration.
	_, _ = privateControl(plan.DataDir, plan.Identity, "DELETE", plan.Token)
	if plan.Deployment.Kind == "systemd" {
		// Do not stop a service whose maintenance gate has already been opened.
		var current selfupdate.Plan
		if selfupdate.ReadJSON(filepath.Join(filepath.Dir(plan.Root), "pending.json"), &current) != nil || current.Phase == "complete" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, "systemctl", "stop", plan.Unit).Run()
	}
}
func cleanupOldTransactions(plan selfupdate.Plan) {
	items, _ := os.ReadDir(filepath.Dir(plan.Root))
	for _, item := range items {
		if !item.IsDir() || !strings.HasPrefix(item.Name(), "transaction-") || item.Name() == plan.ID {
			continue
		}
		path := filepath.Join(filepath.Dir(plan.Root), item.Name())
		var old selfupdate.Plan
		if selfupdate.ReadJSON(filepath.Join(path, "plan.json"), &old) != nil || old.Phase != "complete" {
			continue
		}
		if old.Root != path || old.ID != item.Name() {
			continue
		}
		if selfupdate.NoSymlinks(path) == nil {
			_ = os.RemoveAll(path)
		}
	}
}

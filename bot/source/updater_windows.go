package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func hideUpdateHelper(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}
func atomicReplaceExecutable(source, target string) error {
	from, e := windows.UTF16PtrFromString(source)
	if e != nil {
		return e
	}
	to, e := windows.UTF16PtrFromString(target)
	if e != nil {
		return e
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
func copyAndReplace(source, target string) error {
	info, e := os.Lstat(target)
	if e == nil && !info.Mode().IsRegular() {
		return errors.New("Replacement target is not a regular file")
	}
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	tmp, e := os.CreateTemp(filepath.Dir(target), ".mubble-replace-*.exe")
	if e != nil {
		return e
	}
	name := tmp.Name()
	tmp.Close()
	os.Remove(name)
	defer os.Remove(name)
	if e = copyUpdateFile(source, name); e != nil {
		return e
	}
	return atomicReplaceExecutable(name, target)
}
func parentUpdateProcess(p UpdatePlan) (windows.Handle, error) {
	handle, e := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(p.ParentPID))
	if e != nil {
		return 0, e
	}
	buf := make([]uint16, 32768)
	size := uint32(len(buf))
	e = windows.QueryFullProcessImageName(handle, 0, &buf[0], &size)
	actual := windows.UTF16ToString(buf[:size])
	clean := func(path string) string { return strings.TrimPrefix(filepath.Clean(path), `\\?\`) }
	if e != nil || !strings.EqualFold(clean(actual), clean(p.Target)) {
		windows.CloseHandle(handle)
		return 0, errors.New("Original bot process did not match the update target")
	}
	return handle, nil
}
func startUpdatedBot(p UpdatePlan, healthPath string) (*exec.Cmd, error) {
	args := []string{}
	if p.ResumeBot {
		args = append(args, "--resume-bot")
	}
	if healthPath != "" {
		args = append(args, "--update-health", healthPath)
	}
	cmd := exec.Command(p.Target, args...)
	cmd.Dir = filepath.Dir(p.Target)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
	e := cmd.Start()
	return cmd, e
}
func awaitUpdateHealth(cmd *exec.Cmd, path string, p UpdatePlan) error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(45 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b, _ := os.ReadFile(path)
			var health UpdateHealth
			if json.Unmarshal(b, &health) == nil && health.Version == p.Version && health.Nonce == p.Nonce {
				return nil
			}
		case <-done:
			return errors.New("The new executable exited before opening its panel")
		case <-timer.C:
			cmd.Process.Kill()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
			}
			return errors.New("The new executable did not pass its startup check")
		}
	}
}
func runUpdateHelper(planPath string) (err error) {
	p, appDir, e := loadUpdatePlan(planPath)
	if e != nil {
		return e
	}
	activePID := 0
	defer func() {
		r := UpdateResult{Version: p.Version, Success: err == nil, Time: time.Now().Format(time.RFC3339), PID: activePID}
		if err != nil {
			r.Error = err.Error()
		}
		atomicJSON(filepath.Join(appDir, "update-result.json"), r)
	}()
	if !newerVersion(p.Version, version) {
		return errors.New("Refusing to install an equal or older version")
	}
	if e = verifyUpdateFile(p.Staged, p.SHA256, p.Size); e != nil {
		return e
	}
	parent, e := parentUpdateProcess(p)
	if e != nil {
		return e
	}
	defer windows.CloseHandle(parent)
	if e = atomicJSON(filepath.Join(filepath.Dir(planPath), "ready.json"), UpdateHealth{Nonce: p.Nonce}); e != nil {
		return e
	}
	status, e := windows.WaitForSingleObject(parent, 180000)
	if e != nil || status != windows.WAIT_OBJECT_0 {
		return errors.New("The original bot did not exit; executable unchanged")
	}
	unlock, e := lockInstance(filepath.Join(appDir, "instance.lock"))
	if e != nil {
		return errors.New("Another bot instance started; update cancelled")
	}
	locked := true
	defer func() {
		if locked {
			unlock()
		}
	}()
	if e = verifyUpdateFile(p.Staged, p.SHA256, p.Size); e != nil {
		return e
	}
	backup := p.Target + ".previous"
	if e = copyAndReplace(p.Target, backup); e != nil {
		return fmt.Errorf("Cannot keep the previous executable: %w", e)
	}
	if e = copyAndReplace(p.Staged, p.Target); e != nil {
		unlock()
		locked = false
		restarted, restartErr := startUpdatedBot(p, "")
		if restartErr == nil {
			activePID = restarted.Process.Pid
		}
		return fmt.Errorf("Cannot replace the executable; previous version kept: %w", e)
	}
	unlock()
	locked = false
	healthPath := filepath.Join(filepath.Dir(planPath), "health.json")
	cmd, e := startUpdatedBot(p, healthPath)
	if e == nil {
		activePID = cmd.Process.Pid
		e = awaitUpdateHealth(cmd, healthPath, p)
	}
	if e != nil {
		unlockRollback, lockErr := lockInstance(filepath.Join(appDir, "instance.lock"))
		if lockErr != nil {
			return fmt.Errorf("New version failed; close it and recover %s: %w", backup, e)
		}
		restoreErr := copyAndReplace(backup, p.Target)
		unlockRollback()
		if restoreErr != nil {
			return fmt.Errorf("New version failed and rollback failed; recover %s: %w", backup, restoreErr)
		}
		restarted, restartErr := startUpdatedBot(p, "")
		if restartErr == nil {
			activePID = restarted.Process.Pid
		}
		return fmt.Errorf("New version failed its startup check; previous version restored: %w", e)
	}
	os.Remove(p.Staged)
	return nil
}

package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Runs on the Windows release runner so actual executable replacement, startup
// and rollback are checked, beyond cross-compilation and mocked downloads.
func TestWindowsUpdaterReplacesRestartsAndRollsBack(t *testing.T) {
	if os.Getenv("MUBBLE_WINDOWS_UPDATE_TEST") != "1" {
		t.Skip("Enable MUBBLE_WINDOWS_UPDATE_TEST for native Windows installation checks")
	}
	for _, failNew := range []bool{false, true} {
		name := "install"
		if failNew {
			name = "rollback"
		}
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			t.Setenv("APPDATA", base)
			t.Setenv("MUBBLE_NO_BROWSER", "1")
			appDir := filepath.Join(base, "MubbleDiscordBot")
			os.MkdirAll(appDir, 0700)
			cfg := defaultConfig()
			cfg.AutoStart = false
			cfg.AutoUpdate = false
			if e := atomicJSON(filepath.Join(appDir, "config.json"), cfg); e != nil {
				t.Fatal(e)
			}
			dataPath := filepath.Join(appDir, "state.json")
			state := SavedState{Warnings: map[string][]Warning{"kept": {{ID: "1", Reason: "keep saved data"}}}}
			if e := atomicJSON(dataPath, state); e != nil {
				t.Fatal(e)
			}
			before, _ := os.ReadFile(dataPath)
			target := filepath.Join(base, "MubbleDiscordBot.exe")
			build := exec.Command("go", "build", "-buildvcs=false", "-o", target, ".")
			if out, e := build.CombinedOutput(); e != nil {
				t.Fatal(e, string(out))
			}
			oldBytes, _ := os.ReadFile(target)
			oldHash := sha256.Sum256(oldBytes)
			dir := filepath.Join(appDir, "updates", "pending-native")
			os.MkdirAll(dir, 0700)
			staged := filepath.Join(dir, "update.exe")
			if failNew {
				os.WriteFile(staged, sampleUpdatePE(), 0600)
			} else {
				build = exec.Command("go", "build", "-buildvcs=false", "-ldflags=-X main.version="+nextUpdateVersion(), "-o", staged, ".")
				if out, e := build.CombinedOutput(); e != nil {
					t.Fatal(e, string(out))
				}
			}
			updated, _ := os.ReadFile(staged)
			hash := sha256.Sum256(updated)
			helper := filepath.Join(dir, "helper.exe")
			if e := copyUpdateFile(target, helper); e != nil {
				t.Fatal(e)
			}
			parent := exec.Command(target)
			stdout, e := parent.StdoutPipe()
			if e != nil {
				t.Fatal(e)
			}
			if e = parent.Start(); e != nil {
				t.Fatal(e)
			}
			defer parent.Process.Kill()
			opened := make(chan bool, 1)
			go func() {
				scan := bufio.NewScanner(stdout)
				for scan.Scan() {
					if strings.HasPrefix(scan.Text(), "Control panel:") {
						opened <- true
						return
					}
				}
			}()
			select {
			case <-opened:
			case <-time.After(20 * time.Second):
				t.Fatal("old executable did not open panel")
			}
			plan := UpdatePlan{Target: target, Staged: staged, ParentPID: parent.Process.Pid, SHA256: hex.EncodeToString(hash[:]), Size: int64(len(updated)), Version: nextUpdateVersion(), Nonce: strings.Repeat("a", 64)}
			planPath := filepath.Join(dir, "plan.json")
			atomicJSON(planPath, plan)
			command := exec.Command(helper, "--apply-update", planPath)
			if e = command.Start(); e != nil {
				t.Fatal(e)
			}
			defer command.Process.Kill()
			ready := filepath.Join(dir, "ready.json")
			deadline := time.Now().Add(20 * time.Second)
			for time.Now().Before(deadline) {
				if _, e = os.Stat(ready); e == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			if _, e = os.Stat(ready); e != nil {
				t.Fatal("helper did not bind original process")
			}
			parent.Process.Kill()
			parent.Wait()
			done := make(chan error, 1)
			go func() { done <- command.Wait() }()
			select {
			case e = <-done:
				if !failNew && e != nil {
					t.Fatal(e)
				}
				if failNew && e == nil {
					t.Fatal("invalid executable accepted")
				}
			case <-time.After(65 * time.Second):
				t.Fatal("native update did not finish")
			}
			result := readUpdateResult(appDir)
			if result.PID > 0 {
				proc, _ := os.FindProcess(result.PID)
				defer proc.Kill()
			}
			if failNew && (result.Success || !strings.Contains(result.Error, "previous version restored")) {
				t.Fatal(result)
			}
			if !failNew && !result.Success {
				t.Fatal(result)
			}
			actual, _ := os.ReadFile(target)
			actualHash := sha256.Sum256(actual)
			wanted := hash
			if failNew {
				wanted = oldHash
			}
			if actualHash != wanted {
				t.Fatal("wrong executable after update")
			}
			backup, _ := os.ReadFile(target + ".previous")
			if sha256.Sum256(backup) != oldHash {
				t.Fatal("previous executable backup did not match")
			}
			after, _ := os.ReadFile(dataPath)
			if string(before) != string(after) {
				t.Fatal("update changed saved state")
			}
			if !failNew {
				var health UpdateHealth
				b, _ := os.ReadFile(filepath.Join(dir, "health.json"))
				json.Unmarshal(b, &health)
				if health.Version != nextUpdateVersion() || health.Nonce != plan.Nonce {
					t.Fatal("new executable did not confirm startup")
				}
			}
		})
	}
}

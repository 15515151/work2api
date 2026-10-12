package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"work2api/internal/selfupdate"
)

func TestLaunchArgsPreservesControlLikeValues(t *testing.T) {
	opt, remaining := launchArgs([]string{"-admin-token", "-stop-running", "--host", "-open-browser", "-reuse-only", "--", "-build-info"})
	if opt.stop || opt.open || opt.info || !opt.reuse || strings.Join(remaining, "|") != "-admin-token|-stop-running|--host|-open-browser|--|-build-info" {
		t.Fatalf("control flags consumed configuration values: %+v %q", opt, remaining)
	}
}

func TestDrainTimeoutPreservesService(t *testing.T) {
	g := &drainGate{}
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	h := g.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release; w.WriteHeader(200) }))
	go func() {
		defer close(finished)
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	}()
	<-entered
	if err := g.drain(context.Background(), 5*time.Millisecond); err == nil {
		t.Fatal("active request bypassed")
	}
	close(release)
	<-finished
	w := httptest.NewRecorder()
	g.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 {
		t.Fatal("old service stayed closed", w.Code)
	}
	if err := g.drain(context.Background(), time.Second); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 503 {
		t.Fatal("new request entered draining service")
	}
}

func TestControlIdentityAuthorizationAndGracefulStop(t *testing.T) {
	dir := t.TempDir()
	identity := "isolated-instance"
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"status":"ok"}`)) }))
	defer health.Close()
	stop := make(chan struct{}, 1)
	cleanup, err := startControl(dir, identity, health.URL+"/admin-ui/", &drainGate{}, stop)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	info, err := contactInstance(dir, identity, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := contactInstance(dir, "different-state", false); err == nil {
		t.Fatal("reused other state")
	}
	for _, origin := range []string{"", "http://untrusted.example"} {
		r, _ := http.NewRequest("POST", info.Control, nil)
		if origin != "" {
			r.Header.Set("Authorization", "Bearer "+info.Token)
			r.Header.Set("Origin", origin)
		}
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatal("unauthorized control", resp.StatusCode)
		}
	}
	select {
	case <-stop:
		t.Fatal("unauthorized shutdown")
	default:
	}
	if _, err := contactInstance(dir, identity, true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stop:
	case <-time.After(time.Second):
		t.Fatal("stop not signaled")
	}
	if _, err := contactInstance(dir, identity, false); err == nil {
		t.Fatal("draining service reused")
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".instance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata instanceInfo
	if json.Unmarshal(raw, &metadata) != nil || metadata.Token == "" {
		t.Fatal("metadata invalid")
	}
}

func TestInstanceProcessLock(t *testing.T) {
	if path := os.Getenv("WORK2API_TEST_LOCK"); path != "" {
		release, err := lockInstance([]string{path})
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		fmt.Println("locked")
		_, _ = bufio.NewReader(os.Stdin).ReadByte()
		return
	}
	path := filepath.Join(t.TempDir(), "state.instance.lock")
	child := exec.Command(os.Args[0], "-test.run=^TestInstanceProcessLock$")
	child.Env = append(os.Environ(), "WORK2API_TEST_LOCK="+path)
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "locked" {
		t.Fatalf("child lock failed: %q %v", line, err)
	}
	if release, err := lockInstance([]string{path}); err == nil {
		release()
		t.Fatal("shared state permitted")
	}
	_ = child.Process.Kill()
	_ = child.Wait()
	release, err := lockInstance([]string{path})
	if err != nil {
		t.Fatal("crash did not release lock", err)
	}
	defer release()
	other, err := lockInstance([]string{filepath.Join(t.TempDir(), "separate.instance.lock")})
	if err != nil {
		t.Fatal(err)
	}
	other()
}

// TestListenExplicitPortOccupiedFailsLoud: a port the operator set explicitly
// (i.e. not the default 8787) must NOT be auto-avoided — listen returns an
// error so their choice is respected instead of silently drifting.
func TestListenExplicitPortOccupiedFailsLoud(t *testing.T) {
	// Occupy an OS-assigned free port, then demand exactly that port.
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("setup listen: %v", err)
	}
	defer blocker.Close()
	port := blocker.Addr().(*net.TCPAddr).Port
	if port == 8787 { // astronomically unlikely, but keep the test meaningful
		t.Skip("OS handed us the default port")
	}

	ln, _, err := listen("127.0.0.1", port)
	if err == nil {
		ln.Close()
		t.Fatalf("explicit occupied port %d should fail, not avoid", port)
	}
	if !strings.Contains(err.Error(), "已被占用") {
		t.Fatalf("error should explain the port is occupied, got: %v", err)
	}
}

// TestListenBindsFreePort: a free port binds and reports its address back.
func TestListenBindsFreePort(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close() // release it so listen() can grab the same port

	ln, addr, err := listen("127.0.0.1", port)
	if err != nil {
		t.Fatalf("free port %d should bind, got: %v", port, err)
	}
	defer ln.Close()
	if !strings.HasSuffix(addr, ":"+itoa(port)) {
		t.Fatalf("addr %q should end in bound port %d", addr, port)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestReleaseMarkerProtectsSourceBuildsAndRoundTripsPrivatePlan(t *testing.T) {
	original := releaseBuild
	releaseBuild = "false"
	defer func() { releaseBuild = original }()
	info := currentBuild()
	info.Version = "v0.4.0"
	if info.Release {
		t.Fatal("tag-shaped version promoted source build")
	}
	file := filepath.Join(t.TempDir(), "work2api")
	if err := os.WriteFile(file, []byte("source binary"), 0700); err != nil {
		t.Fatal(err)
	}
	deployment := updateDeployment(file, info)
	if deployment.CanInstall || deployment.Kind != "source" {
		t.Fatal("source installation allowed", deployment)
	}
	path := filepath.Join(t.TempDir(), "plan.json")
	plan := selfupdate.Plan{CandidatePath: "private-path", Candidate: selfupdate.Candidate{Path: "private-path"}, Token: "secret", Args: []string{"private-argument"}}
	if err := selfupdate.WriteJSON(path, plan); err != nil {
		t.Fatal(err)
	}
	var decoded selfupdate.Plan
	if err := selfupdate.ReadJSON(path, &decoded); err != nil || decoded.Candidate.Path != "private-path" {
		t.Fatal("candidate path lost during helper handoff", decoded, err)
	}
	public, _ := json.Marshal(selfupdate.Status{Candidate: &plan.Candidate})
	if strings.Contains(string(public), "private-path") || strings.Contains(string(public), "secret") {
		t.Fatal("local-only transaction leaked")
	}
}
func TestUpdateArchiveRejectsTraversalDuplicatesAndLinks(t *testing.T) {
	for _, names := range [][]string{{"../work2api"}, {"folder/" + selfupdate.ExecutableName(), "another/" + selfupdate.ExecutableName()}, {"folder/other"}} {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		for _, name := range names {
			file, err := writer.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			file.Write([]byte("program"))
		}
		writer.Close()
		target := filepath.Join(t.TempDir(), "candidate")
		if err := selfupdate.ExtractBinary(buffer.Bytes(), "zip", target); err == nil {
			t.Fatal("unsafe archive accepted", names)
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatal("unsafe candidate written")
		}
	}
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "folder/" + selfupdate.ExecutableName(), Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}); err != nil {
		t.Fatal(err)
	}
	tarWriter.Close()
	gzipWriter.Close()
	if err := selfupdate.ExtractBinary(buffer.Bytes(), "tar.gz", filepath.Join(t.TempDir(), "candidate")); err == nil {
		t.Fatal("archive symlink accepted")
	}
}
func TestVerifiedBackupRestoresStateBeforeBinaryRollback(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	db := filepath.Join(data, "database.sqlite")
	os.MkdirAll(data, 0700)
	files := map[string][]byte{"database.sqlite": []byte("old database"), "sources/1/credential.json": []byte("old secret"), "effective-config.json": []byte("private config")}
	sources := map[string]string{"database.sqlite": db, "sources/1": data, "effective-config.json": "effective application configuration"}
	hashes := map[string]string{}
	for name, content := range files {
		sum := sha256.Sum256(content)
		hashes[name] = hex.EncodeToString(sum[:])
	}
	manifest, _ := json.Marshal(map[string]any{"format_version": 1, "sources": sources, "sha256": hashes})
	files["manifest.json"] = manifest
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		tw.Write(content)
	}
	tw.Close()
	gz.Close()
	backup := filepath.Join(root, "rollback.tar.gz")
	os.WriteFile(backup, buffer.Bytes(), 0600)
	digest, _ := selfupdate.HashFile(backup)
	os.WriteFile(db, []byte("migrated database"), 0600)
	os.WriteFile(db+"-wal", []byte("new wal"), 0600)
	os.WriteFile(filepath.Join(data, "new-state.json"), []byte("new state"), 0600)
	os.MkdirAll(filepath.Join(data, ".updates"), 0700)
	os.WriteFile(filepath.Join(data, ".updates", "keep"), []byte("helper"), 0600)
	if err := selfupdate.RestoreBackup(backup, strings.Repeat("0", 64), filepath.Join(root, "work2api"), data, db); err == nil {
		t.Fatal("bad checksum accepted")
	}
	if content, _ := os.ReadFile(db); string(content) != "migrated database" {
		t.Fatal("invalid backup modified live data")
	}
	if err := selfupdate.RestoreBackup(backup, digest, filepath.Join(root, "work2api"), data, db); err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(db); string(content) != "old database" {
		t.Fatal("snapshot not restored")
	}
	if content, _ := os.ReadFile(filepath.Join(data, "credential.json")); string(content) != "old secret" {
		t.Fatal("credential not restored")
	}
	for _, path := range []string{db + "-wal", filepath.Join(data, "new-state.json")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("new schema state survived rollback", path)
		}
	}
	if content, _ := os.ReadFile(filepath.Join(data, ".updates", "keep")); string(content) != "helper" {
		t.Fatal("updater was deleted")
	}
}

type updateRoundTripper func(*http.Request) (*http.Response, error)

func (f updateRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestUpdateDownloadChecksumFailureNeverTouchesRunningBinary(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "work2api")
	os.WriteFile(target, []byte("old binary"), 0700)
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	name := "work2api_v0.4.0_" + runtime.GOOS + "_" + runtime.GOARCH + "." + ext
	repo := "owner/repo"
	base := "https://github.com/" + repo + "/releases/download/v0.4.0/"
	release := selfupdate.Release{Tag: "v0.4.0", Assets: []selfupdate.Asset{{Name: name, URL: base + name, Size: 3}, {Name: name + ".sha256", URL: base + name + ".sha256"}}}
	raw, _ := json.Marshal(release)
	oldTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = oldTransport }()
	http.DefaultTransport = updateRoundTripper(func(r *http.Request) (*http.Response, error) {
		content := raw
		if strings.HasSuffix(r.URL.Path, ".sha256") {
			content = []byte(strings.Repeat("0", 64) + "  " + name + "\n")
		} else if r.URL.Host == "github.com" {
			content = []byte("bad")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(content))}, nil
	})
	launched := false
	manager := selfupdate.New(filepath.Join(dir, ".updates"), target, selfupdate.Build{Version: "v0.4.0", Repository: repo, Release: true}, selfupdate.Deployment{Kind: "binary", CanInstall: true}, func(string) error { launched = true; return nil }, func() selfupdate.Plan { return selfupdate.Plan{} })
	if err := manager.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !manager.Status().SameVersion {
		t.Fatal("republished version ignored")
	}
	if err := manager.Download(context.Background()); err == nil {
		t.Fatal("bad archive checksum accepted")
	}
	if err := manager.Install(false); err == nil || launched {
		t.Fatal("unverified candidate reached updater")
	}
	if content, _ := os.ReadFile(target); string(content) != "old binary" {
		t.Fatal("old binary changed")
	}
	source := selfupdate.New(filepath.Join(dir, "source-updates"), target, selfupdate.Build{Repository: repo}, selfupdate.Deployment{Kind: "source", Reason: "source"}, nil, nil)
	if err := source.SetPolicy(selfupdate.Policy{Mode: "auto", EveryHours: 6, StartHour: 2, EndHour: 4}); err == nil {
		t.Fatal("source enabled automatic installation")
	}
}
func TestUpdateControlBackupFailureReopensOldService(t *testing.T) {
	dir := t.TempDir()
	gate := &drainGate{}
	stop := make(chan struct{}, 1)
	cleanup, err := startControl(dir, "test", "http://127.0.0.1:8787/admin-ui/", gate, stop, updateHooks{prepare: func(context.Context, string) error { return fmt.Errorf("backup failed") }})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := privateControl(dir, "test", "POST", "transaction"); err == nil {
		t.Fatal("backup failure accepted")
	}
	select {
	case <-stop:
		t.Fatal("backup failure stopped old instance")
	default:
	}
	w := httptest.NewRecorder()
	gate.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 {
		t.Fatal("old service stayed drained")
	}
}

func TestIsolatedReleaseUpdateProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("isolated release process requires binary builds")
	}
	root := t.TempDir()
	module, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, selfupdate.ExecutableName())
	updates := filepath.Join(root, "data", ".updates")
	candidateDir := filepath.Join(updates, "candidate-fixture")
	os.MkdirAll(candidateDir, 0700)
	candidate := filepath.Join(candidateDir, selfupdate.ExecutableName())
	build := func(path, version, commit string) {
		t.Helper()
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-X main.releaseBuild=true -X main.releaseRepository=owner/repo -X main.version="+version+" -X main.releaseCommit="+commit, "-o", path, "./cmd/server")
		cmd.Dir = module
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated build: %s %v", output, err)
		}
	}
	build(target, "v9.0.0", "old")
	build(candidate, "v9.0.1", "new")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close()
	home := filepath.Join(root, "home")
	os.MkdirAll(home, 0700)
	env := []string{}
	for _, entry := range os.Environ() {
		key := strings.SplitN(entry, "=", 2)[0]
		if key == "HOME" || key == "USERPROFILE" || key == "APPDATA" || key == "LOCALAPPDATA" || strings.HasPrefix(key, "QODER") || strings.HasPrefix(key, "OPENCODE") || key == "ENV_FILE" || key == "BACKUP_EXTRA_PATHS" || key == "INVOCATION_ID" || key == "SYSTEMD_EXEC_PID" || key == "ADMIN_TOKEN" || key == "WORK2API_CONTAINER" || key == "XDG_DATA_HOME" {
			continue
		}
		env = append(env, entry)
	}
	env = append(env, "HOME="+home, "USERPROFILE="+home, "APPDATA="+home, "LOCALAPPDATA="+home, "XDG_DATA_HOME="+home, "QODER2API_HOME="+filepath.Join(root, "qoder"), "LOG_TO_FILE=0")
	data := filepath.Join(root, "data")
	db := filepath.Join(data, "work2api.db")
	args := []string{"-host", "127.0.0.1", "-port", port, "-strict-port", "-data-dir", data, "-db", db}
	logFile, err := os.Create(filepath.Join(root, "isolated.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	command := exec.Command(target, args...)
	command.Dir = root
	command.Env = env
	command.Stdout = logFile
	command.Stderr = logFile
	detachUpdate(command)
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	go command.Wait()
	t.Cleanup(func() {
		var info instanceInfo
		if selfupdate.ReadJSON(filepath.Join(data, ".instance.json"), &info) == nil && info.PID > 0 {
			process, _ := os.FindProcess(info.PID)
			if process != nil {
				_ = process.Kill()
			}
		}
		_ = command.Process.Kill()
		time.Sleep(300 * time.Millisecond)
	})
	var info instanceInfo
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if selfupdate.ReadJSON(filepath.Join(data, ".instance.json"), &info) == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if info.PID == 0 {
		raw, _ := os.ReadFile(filepath.Join(root, "isolated.log"))
		t.Fatalf("isolated old server failed: %s", raw)
	}
	current, err := selfupdate.Probe(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	incoming, err := selfupdate.Probe(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	transaction := filepath.Join(updates, "transaction-fixture")
	os.MkdirAll(transaction, 0700)
	helper := filepath.Join(transaction, "helper"+filepath.Ext(selfupdate.ExecutableName()))
	if err = selfupdate.CopyFile(target, helper, 0700); err != nil {
		t.Fatal(err)
	}
	oldHash, _ := selfupdate.HashFile(target)
	newHash, _ := selfupdate.HashFile(candidate)
	plan := selfupdate.Plan{ID: "transaction-fixture", Token: strings.Repeat("b", 64), Root: transaction, Target: target, OldHash: oldHash, Candidate: selfupdate.Candidate{Build: incoming, BinaryHash: newHash}, CandidatePath: candidate, Current: current, Deployment: selfupdate.Deployment{Kind: "binary", CanInstall: true}, DataDir: data, DBPath: db, CWD: root, Args: args, Identity: info.Identity, Phase: "installing", Started: time.Now()}
	path := filepath.Join(transaction, "plan.json")
	if err = selfupdate.WriteJSON(path, plan); err != nil {
		t.Fatal(err)
	}
	if err = selfupdate.WriteJSON(filepath.Join(updates, "pending.json"), plan); err != nil {
		t.Fatal(err)
	}
	updater := exec.Command(helper, "-apply-update", path)
	updater.Dir = root
	updater.Env = env
	if output, err := updater.CombinedOutput(); err != nil {
		logs, _ := os.ReadFile(filepath.Join(transaction, "service-start.log"))
		t.Fatalf("isolated upgrade: %v %s %s", err, output, logs)
	}
	var final selfupdate.Plan
	if err = selfupdate.ReadJSON(filepath.Join(updates, "pending.json"), &final); err != nil || final.Phase != "complete" {
		t.Fatal("update did not commit", final.Phase, err)
	}
	if final.BackupHash == "" {
		t.Fatal("update did not pin full backup")
	}
	response, err := http.Get("http://127.0.0.1:" + port + "/health")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("new service stayed gated", response.StatusCode)
	}
	var live instanceInfo
	if err = selfupdate.ReadJSON(filepath.Join(data, ".instance.json"), &live); err != nil || live.Version != "v9.0.1" || live.PID == info.PID {
		t.Fatal("new instance not verified", live, err)
	}
}

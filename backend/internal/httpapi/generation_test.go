package httpapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	oexec "os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	"campaigncomposer/internal/httpapi"
	"campaigncomposer/internal/llm"

	"github.com/jackc/pgx/v5/pgxpool"
)

// heldContent is a mock LLM whose Content call waits until release is
// called (or its request ends), then writes the default mock's Content.
func heldContent(t *testing.T) (mock *llm.Mock, release func()) {
	t.Helper()
	gate := make(chan struct{})
	release = sync.OnceFunc(func() { close(gate) })
	t.Cleanup(release)
	return &llm.Mock{ContentFunc: func(ctx context.Context, req llm.ContentRequest, emit func(string)) error {
		select {
		case <-gate:
		case <-ctx.Done():
			return ctx.Err()
		}
		return llm.NewMock().Content(ctx, req, emit)
	}}, release
}

// openContentStream starts the Content call and returns once its stream is
// open, which is when the generation has started.
func openContentStream(t *testing.T, baseURL string, id int64) *http.Response {
	t.Helper()
	res, err := http.Post(baseURL+"/api/campaigns/"+strconv.FormatInt(id, 10)+"/content", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST content: status code %d, want 200", res.StatusCode)
	}
	return res
}

func TestAGenerationRunningLongerThanItsHeartbeatStillBlocksApproval(t *testing.T) {
	mock, release := heldContent(t)
	heartbeat := 50 * time.Millisecond
	srv, pool := startAPIWithPool(t, httpapi.Options{LLM: mock, GenerationHeartbeat: heartbeat})
	c := createCampaign(t, srv.URL, exampleBrief)
	res := openContentStream(t, srv.URL, c.ID)

	// Many heartbeats later the call is still running, so it still counts.
	time.Sleep(20 * heartbeat)
	code, a := approve(t, srv.URL, c.ID)
	assertBlocked(t, pool, c.ID, code, a, "generating")

	release()
	lastDone(t, readSSE(t, res.Body))
	if code, a := approve(t, srv.URL, c.ID); code != http.StatusOK {
		t.Errorf("approve after generation: status code %d, body %+v; want 200", code, a)
	}
}

// The api-process test runs the API in a child process (this test binary
// re-executed into TestAPIProcess), so it can be killed mid-generation the
// way a crashed container would be.

const apiProcessEnv = "CRASH_TEST_API"

// TestAPIProcess is not a test: it is the body of an api process started by
// startAPIProcess, and is skipped in a normal run. Its Content call never
// ends, and it prints the address it serves on.
func TestAPIProcess(t *testing.T) {
	spec := os.Getenv(apiProcessEnv)
	if spec == "" {
		t.Skip("only runs as an api process started by a crash test")
	}
	var p struct {
		DatabaseURL string
		Heartbeat   time.Duration
	}
	if err := json.Unmarshal([]byte(spec), &p); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(context.Background(), p.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	forever := &llm.Mock{ContentFunc: func(ctx context.Context, _ llm.ContentRequest, _ func(string)) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("listening on %s\n", ln.Addr())
	_ = http.Serve(ln, httpapi.NewHandler(pool, httpapi.Options{LLM: forever, GenerationHeartbeat: p.Heartbeat}))
}

// startAPIProcess runs the API in a child process against the test's
// database and returns its base URL and the process.
func startAPIProcess(t *testing.T, pool *pgxpool.Pool, heartbeat time.Duration) (string, *workerProcess) {
	t.Helper()
	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + pool.Config().ConnConfig.Database
	spec, _ := json.Marshal(map[string]any{"DatabaseURL": u.String(), "Heartbeat": heartbeat})

	cmd := oexec.Command(os.Args[0], "-test.run=^TestAPIProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), apiProcessEnv+"="+string(spec))
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &workerProcess{cmd: cmd, done: make(chan struct{})}
	// Wait closes the pipe only once the process has exited.
	go func() { _ = cmd.Wait(); close(p.done) }()
	t.Cleanup(p.kill)
	var addr string
	if _, err := fmt.Fscanf(bufio.NewReader(out), "listening on %s\n", &addr); err != nil {
		t.Fatalf("read api process address: %v", err)
	}
	return "http://" + addr, p
}

func TestAGenerationLeftByACrashedAPIStopsBlockingApprovalOnceItsHeartbeatStops(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	heartbeat := 100 * time.Millisecond
	crashedURL, crashed := startAPIProcess(t, pool, heartbeat)
	openContentStream(t, crashedURL, c.ID)
	code, a := approve(t, baseURL, c.ID)
	assertBlocked(t, pool, c.ID, code, a, "generating")

	crashed.kill() // no chance to end its generation
	killed := time.Now()

	waitUntil(t, "approval goes through", func() bool {
		code, _ := approve(t, baseURL, c.ID)
		return code == http.StatusOK
	})
	if waited := time.Since(killed); waited > 10*heartbeat {
		t.Errorf("approval went through %v after the crash, want within a few heartbeats of %v", waited, heartbeat)
	}
}

package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"campaigncomposer/internal/provider"
	"campaigncomposer/internal/worker"

	"github.com/jackc/pgx/v5/pgxpool"
)

// startScriptedProvider serves the mock Provider with every request's
// outcome pinned by script.
func startScriptedProvider(t *testing.T, script provider.Script) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(provider.New(provider.Options{Script: script}))
	t.Cleanup(srv.Close)
	return srv
}

// fastRetries keeps backoff and the client timeout small so retry tests run
// in milliseconds. The rate limit is set far out of reach: these scripts
// throttle every Delivery, and the 429 slow-down (#14) would otherwise
// stretch the test to minutes; pacing has its own tests.
func fastRetries(providerURL string) worker.Options {
	return worker.Options{
		ProviderURL:  providerURL,
		PollInterval: 5 * time.Millisecond,
		BackoffBase:  time.Millisecond,
		Timeout:      100 * time.Millisecond,
		RateLimit:    1e6,
	}
}

type attempt struct {
	Number     int       `json:"number"`
	StatusCode *int      `json:"status_code"`
	LatencyMS  int64     `json:"latency_ms"`
	Error      string    `json:"error"`
	Outcome    string    `json:"outcome"`
	At         time.Time `json:"at"`
}

type deliveryWithAttempts struct {
	ID           int64     `json:"id"`
	Channel      string    `json:"channel"`
	State        string    `json:"state"`
	AttemptCount int       `json:"attempt_count"`
	Attempts     []attempt `json:"attempts"`
}

// campaignDeliveries lists the Campaign's Deliveries in state with their
// Attempt log, the record an operator reads to explain a Delivery (story 67).
func campaignDeliveries(t *testing.T, pool *pgxpool.Pool, id int64, state string) []deliveryWithAttempts {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT d.id, d.channel, d.state, d.attempt_count,
			COALESCE((SELECT json_agg(json_build_object(
					'number', a.number, 'status_code', a.status_code, 'latency_ms', a.latency_ms,
					'error', a.error, 'outcome', a.outcome, 'at', a.started_at) ORDER BY a.number, a.id)
				FROM attempts a WHERE a.delivery_id = d.id), '[]')
		FROM deliveries d WHERE d.campaign_id = $1 AND d.state = $2 ORDER BY d.id`, id, state)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []deliveryWithAttempts
	for rows.Next() {
		var d deliveryWithAttempts
		var attempts []byte
		if err := rows.Scan(&d.ID, &d.Channel, &d.State, &d.AttemptCount, &attempts); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(attempts, &d.Attempts); err != nil {
			t.Fatal(err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestThrottledThenFailingThenAcceptedSendIsSentAfterThreeAttemptsHonouringRetryAfter(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	prov := startScriptedProvider(t, func(channel, key string, request int) provider.Outcome {
		switch request {
		case 1:
			return provider.Outcome{Code: http.StatusTooManyRequests, RetryAfter: "0.2"}
		case 2:
			return provider.Outcome{Code: http.StatusServiceUnavailable}
		}
		return provider.Outcome{}
	})
	startWorkersWith(t, pool, fastRetries(prov.URL), 1)

	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	got := waitForStatus(t, baseURL, c.ID, "completed")

	if got.Deliveries["sms"].Sent != c.Audience.SMS || got.Deliveries["email"].Sent != c.Audience.Email {
		t.Fatalf("Deliveries = %+v, want all sent", got.Deliveries)
	}
	sent := campaignDeliveries(t, pool, c.ID, "sent")
	if len(sent) == 0 {
		t.Fatal("no sent Deliveries listed")
	}
	for _, d := range sent {
		if d.AttemptCount != 3 || len(d.Attempts) != 3 {
			t.Fatalf("Delivery %d: %d Attempts (%d logged), want 3", d.ID, d.AttemptCount, len(d.Attempts))
		}
		wantCodes := []int{429, 503, 202}
		wantOutcomes := []string{"retry", "retry", "sent"}
		for i, a := range d.Attempts {
			if a.Number != i+1 || a.StatusCode == nil || *a.StatusCode != wantCodes[i] || a.Outcome != wantOutcomes[i] {
				t.Errorf("Delivery %d Attempt %d = %+v, want #%d %d %s", d.ID, i, a, i+1, wantCodes[i], wantOutcomes[i])
			}
		}
		if gap := d.Attempts[1].At.Sub(d.Attempts[0].At); gap < 200*time.Millisecond {
			t.Errorf("Delivery %d retried %v after the 429, want at least its Retry-After of 200ms", d.ID, gap)
		}
	}
}

// assertAllFailedAfter checks that every Delivery of the Campaign failed
// after n Attempts, each logged with wantCode, the last one as `failed`.
func assertAllFailedAfter(t *testing.T, baseURL string, pool *pgxpool.Pool, c campaign, n, wantCode int) {
	t.Helper()
	got := waitForStatus(t, baseURL, c.ID, "completed")
	if got.Deliveries["sms"].Failed != c.Audience.SMS || got.Deliveries["email"].Failed != c.Audience.Email {
		t.Fatalf("Deliveries = %+v, want all failed", got.Deliveries)
	}
	failed := campaignDeliveries(t, pool, c.ID, "failed")
	if len(failed) == 0 {
		t.Fatal("no failed Deliveries listed")
	}
	for _, d := range failed {
		if d.AttemptCount != n || len(d.Attempts) != n {
			t.Fatalf("Delivery %d: %d Attempts (%d logged), want %d", d.ID, d.AttemptCount, len(d.Attempts), n)
		}
		for i, a := range d.Attempts {
			wantOutcome := "retry"
			if i == n-1 {
				wantOutcome = "failed"
			}
			if a.StatusCode == nil || *a.StatusCode != wantCode || a.Outcome != wantOutcome || a.Error == "" {
				t.Errorf("Delivery %d Attempt %d = %+v, want %d %s with an error", d.ID, i+1, a, wantCode, wantOutcome)
			}
		}
	}
}

func TestFiveRetryableFailuresInARowFailTheDelivery(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	prov := startScriptedProvider(t, func(channel, key string, request int) provider.Outcome {
		return provider.Outcome{Code: http.StatusBadGateway}
	})
	startWorkersWith(t, pool, fastRetries(prov.URL), 1)

	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}

	assertAllFailedAfter(t, baseURL, pool, c, 5, http.StatusBadGateway)
}

func TestAPermanentClientErrorFailsTheDeliveryAfterOneAttempt(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	prov := startScriptedProvider(t, func(channel, key string, request int) provider.Outcome {
		return provider.Outcome{Code: http.StatusUnprocessableEntity}
	})
	startWorkersWith(t, pool, fastRetries(prov.URL), 1)

	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}

	assertAllFailedAfter(t, baseURL, pool, c, 1, http.StatusUnprocessableEntity)
}

func TestASendThatTimesOutAfterTheProviderAcceptedItIsNotDuplicated(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	opts := fastRetries("")
	opts.Timeout = 30 * time.Millisecond
	opts.Concurrency = 64
	prov := startScriptedProvider(t, func(channel, key string, request int) provider.Outcome {
		return provider.Outcome{Delay: 3 * opts.Timeout} // accepted, answered too late
	})
	opts.ProviderURL = prov.URL
	startWorkersWith(t, pool, opts, 1)

	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	got := waitForStatus(t, baseURL, c.ID, "completed")

	if got.Deliveries["sms"].Sent != c.Audience.SMS || got.Deliveries["email"].Sent != c.Audience.Email {
		t.Fatalf("Deliveries = %+v, want all sent", got.Deliveries)
	}
	msgs := providerMessages(t, prov.URL)
	if len(msgs) != c.Audience.SMS+c.Audience.Email {
		t.Errorf("Provider recorded %d messages, want one per Delivery (%d)", len(msgs), c.Audience.SMS+c.Audience.Email)
	}
	for _, m := range msgs {
		if m.Requests != 2 {
			t.Errorf("Delivery %s was requested %d times, want 2 (the timeout, then the replayed acceptance)", m.IdempotencyKey, m.Requests)
		}
	}
	for _, d := range campaignDeliveries(t, pool, c.ID, "sent") {
		if len(d.Attempts) != 2 {
			t.Fatalf("Delivery %d logged %d Attempts, want 2", d.ID, len(d.Attempts))
		}
		first, second := d.Attempts[0], d.Attempts[1]
		if first.StatusCode != nil || first.Error == "" || first.Outcome != "retry" || first.LatencyMS < opts.Timeout.Milliseconds() {
			t.Errorf("Delivery %d first Attempt = %+v, want a timeout with no status code after %v", d.ID, first, opts.Timeout)
		}
		if second.StatusCode == nil || *second.StatusCode != http.StatusAccepted || second.Outcome != "sent" {
			t.Errorf("Delivery %d second Attempt = %+v, want 202 sent", d.ID, second)
		}
	}
}

// metrics scrapes /metrics and returns each sample by its series, e.g.
// `campaign_deliveries_sent_total{channel="sms"}`.
func metrics(t *testing.T, baseURL string) map[string]float64 {
	t.Helper()
	res, err := http.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/plain") {
		t.Fatalf("GET /metrics: status code %d, Content-Type %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	body, _ := io.ReadAll(res.Body)
	out := map[string]float64{}
	for _, line := range strings.Split(string(body), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.LastIndexByte(line, ' ')
		v, err := strconv.ParseFloat(line[i+1:], 64)
		if i < 0 || err != nil {
			t.Fatalf("malformed sample %q", line)
		}
		out[line[:i]] = v
	}
	return out
}

func TestACampaignWithSomeFailedDeliveriesCompletesAndMetricsMatchTheDeliveries(t *testing.T) {
	baseURL, pool, c := reviewedCampaign(t)
	// Even Delivery ids are rejected for good; odd ones are throttled once,
	// then accepted.
	prov := startScriptedProvider(t, func(channel, key string, request int) provider.Outcome {
		id, _ := strconv.Atoi(key)
		switch {
		case id%2 == 0:
			return provider.Outcome{Code: http.StatusBadRequest}
		case request == 1:
			return provider.Outcome{Code: http.StatusTooManyRequests, RetryAfter: "0.01"}
		}
		return provider.Outcome{}
	})
	startWorkersWith(t, pool, fastRetries(prov.URL), 1)

	before := metrics(t, baseURL)
	for _, ch := range []string{"sms", "email"} {
		for _, name := range []string{"campaign_deliveries_sent_total", "campaign_deliveries_failed_total", "campaign_delivery_retries_total"} {
			if v, ok := before[name+`{channel="`+ch+`"}`]; !ok || v != 0 {
				t.Errorf("before sending, %s{channel=%q} = %v (present %t), want 0", name, ch, v, ok)
			}
		}
	}

	if code, a := approve(t, baseURL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}
	got := waitForStatus(t, baseURL, c.ID, "completed")

	m := metrics(t, baseURL)
	for ch, total := range map[string]int{"sms": c.Audience.SMS, "email": c.Audience.Email} {
		counts := got.Deliveries[ch]
		if counts.Sent == 0 || counts.Failed == 0 || counts.Sent+counts.Failed != total || counts.Pending != 0 {
			t.Errorf("%s Deliveries = %+v, want a mix of sent and failed adding up to %d", ch, counts, total)
		}
		want := map[string]int{
			"campaign_deliveries_sent_total":   counts.Sent,
			"campaign_deliveries_failed_total": counts.Failed,
			"campaign_delivery_retries_total":  counts.Sent, // one 429 per sent Delivery
		}
		for name, w := range want {
			series := name + `{channel="` + ch + `"}`
			if m[series] != float64(w) {
				t.Errorf("%s = %v, want %d", series, m[series], w)
			}
		}
	}
}

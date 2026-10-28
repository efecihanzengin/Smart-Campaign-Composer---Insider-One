package httpapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"campaigncomposer/internal/httpapi"
	"campaigncomposer/internal/llm"
)

type dashboardCampaign struct {
	ID         int64                    `json:"id"`
	Brief      string                   `json:"brief"`
	Status     string                   `json:"status"`
	Deliveries map[string]channelCounts `json:"deliveries"`
	Progress   float64                  `json:"progress"`
}

type dashboardEvent struct {
	Campaigns []dashboardCampaign `json:"campaigns"`
}

// dashboardStream reads the dashboard SSE stream one event at a time.
type dashboardStream struct {
	t  *testing.T
	sc *bufio.Scanner
}

func openDashboard(t *testing.T, baseURL string) *dashboardStream {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/dashboard", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET dashboard: %v", err)
	}
	t.Cleanup(func() { res.Body.Close() })
	if ct := res.Header.Get("Content-Type"); res.StatusCode != http.StatusOK || !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("GET dashboard: status code %d, Content-Type %q", res.StatusCode, ct)
	}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	return &dashboardStream{t: t, sc: sc}
}

// next returns the next dashboard event.
func (s *dashboardStream) next() dashboardEvent {
	s.t.Helper()
	var name, data string
	for s.sc.Scan() {
		line := s.sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data += strings.TrimPrefix(line, "data: ")
		case line == "" && name == "dashboard":
			var e dashboardEvent
			if err := json.Unmarshal([]byte(data), &e); err != nil {
				s.t.Fatalf("decode dashboard event: %v", err)
			}
			return e
		case line == "":
			name, data = "", ""
		}
	}
	s.t.Fatalf("dashboard stream ended: %v", s.sc.Err())
	return dashboardEvent{}
}

func (e dashboardEvent) campaign(id int64) (dashboardCampaign, bool) {
	for _, c := range e.Campaigns {
		if c.ID == id {
			return c, true
		}
	}
	return dashboardCampaign{}, false
}

func TestTheDashboardStreamsPerChannelCountsUntilTheCampaignCompletes(t *testing.T) {
	srv, pool := startAPIWithPool(t, httpapi.Options{LLM: llm.NewMock(), DashboardInterval: 20 * time.Millisecond})
	c := createCampaign(t, srv.URL, exampleBrief)
	lastDone(t, streamContent(t, srv.URL, c.ID))
	if code, a := approve(t, srv.URL, c.ID); code != http.StatusOK {
		t.Fatalf("approve: status code %d, body %+v", code, a)
	}

	dash := openDashboard(t, srv.URL)
	first, ok := dash.next().campaign(c.ID)
	if !ok {
		t.Fatalf("the first dashboard event does not show the approved Campaign")
	}
	wantFirst := map[string]channelCounts{
		"sms":   {Pending: c.Audience.SMS, Total: c.Audience.SMS},
		"email": {Pending: c.Audience.Email, Total: c.Audience.Email},
	}
	if first.Status != "approved" || first.Progress != 0 || first.Brief != exampleBrief ||
		first.Deliveries["sms"] != wantFirst["sms"] || first.Deliveries["email"] != wantFirst["email"] {
		t.Errorf("first event = %+v, want approved, all pending, progress 0", first)
	}

	startWorkers(t, pool, startProvider(t).URL, 1)
	updates := 0
	var last dashboardCampaign
	for last.Status != "completed" {
		e := dash.next()
		if got, ok := e.campaign(c.ID); ok {
			last = got
			updates++
		}
	}

	if updates < 2 {
		t.Errorf("got %d updates before completion, want the progress to show in between", updates)
	}
	if last.Progress != 1 || last.Deliveries["sms"].Sent != c.Audience.SMS || last.Deliveries["email"].Sent != c.Audience.Email {
		t.Errorf("last event = %+v, want everything sent and progress 1", last)
	}
}

func TestTheDashboardLeavesOutDraftCampaigns(t *testing.T) {
	srv := startAPI(t, httpapi.Options{LLM: llm.NewMock(), DashboardInterval: 20 * time.Millisecond})
	c := createCampaign(t, srv.URL, exampleBrief)

	if _, ok := openDashboard(t, srv.URL).next().campaign(c.ID); ok {
		t.Errorf("the dashboard shows draft Campaign %d", c.ID)
	}
}

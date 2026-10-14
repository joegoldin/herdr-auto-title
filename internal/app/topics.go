package app

import (
	"time"

	"github.com/kryptamine/herdr-auto-title/internal/herdr"
)

// topicSource is who every topic is reported as. Herdr keys a token by name
// alone, so the source scopes nothing; it is fixed because Herdr limits how
// many sources it keeps track of.
const topicSource = "herdr.auto-title"

// topicTTL is how long Herdr keeps a topic nobody reports again, which is what
// takes a topic down once this stops: nothing is cleared on the way out. See
// docs/architecture/poll-loop.md before changing it or topicRefresh.
const topicTTL = time.Minute

// topicRefresh is how old a report grows before an unchanged topic is sent
// again: a third of topicTTL, so two missed refreshes and a restart still fit.
const topicRefresh = 20 * time.Second

// topicReports remembers what this instance last reported for each workspace.
// The snapshot's tokens cannot say it: they merge every source and carry no
// expiry, and a refresh needs a time only the plugin keeps.
type topicReports struct {
	// token is the workspace token these reports carry.
	token      string
	workspaces map[string]topicReport
	now        func() time.Time
}

// topicReport is the topic last sent to a workspace, "" for a clear, and when.
// A rejected one is not sent again until the topic changes.
type topicReport struct {
	topic    string
	sentAt   time.Time
	rejected bool
}

func newTopicReports(token string) *topicReports {
	return &topicReports{
		token:      token,
		workspaces: make(map[string]topicReport),
		now:        time.Now,
	}
}

// observe forgets the workspaces the session no longer holds.
func (r *topicReports) observe(workspaces []herdr.WorkspaceInfo) {
	ids := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		ids = append(ids, workspace.WorkspaceID)
	}

	r.observeIDs(ids)
}

// observeIDs forgets every resource not in ids. The reports are keyed by ID
// alone, so the same bookkeeping serves workspaces and panes.
func (r *topicReports) observeIDs(ids []string) {
	live := make(map[string]topicReport, len(ids))
	for _, id := range ids {
		if report, known := r.workspaces[id]; known {
			live[id] = report
		}
	}

	r.workspaces = live
}

// due says whether topic should be reported to the workspace now: it differs
// from what was, or what was is due for a refresh.
func (r *topicReports) due(workspace herdr.WorkspaceInfo, topic string) bool {
	return r.dueFor(workspace.WorkspaceID, workspace.Tokens[r.token], topic)
}

// dueFor is due for any resource: shown is the token as the snapshot shows
// it, which only a predecessor's report could have left.
func (r *topicReports) dueFor(id, shown, topic string) bool {
	last, known := r.workspaces[id]

	switch {
	case !known:
		// A predecessor's topic outlives it by up to topicTTL, so one showing
		// is cleared rather than left to expire.
		return topic != "" || shown != ""
	case topic != last.topic:
		return true
	case topic == "" || last.rejected:
		return false
	default:
		return r.now().Sub(last.sentAt) >= topicRefresh
	}
}

func (r *topicReports) sent(id, topic string) {
	r.workspaces[id] = topicReport{topic: topic, sentAt: r.now()}
}

func (r *topicReports) rejected(id, topic string) {
	r.workspaces[id] = topicReport{topic: topic, rejected: true}
}

func (r *topicReports) forget(id string) {
	delete(r.workspaces, id)
}

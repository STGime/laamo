package eurobasetest

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

// Go versions of claim_jobs (0006), extend_lease and finish_job (0007).
// The SQL itself is tested against Postgres in migrations/test; these only
// let the Go queue run against the fake. They run with f.mu held.

func (f *Fake) at() time.Time { return f.clock }

func ts(t time.Time) string { return t.Format(TimeFormat) }

func parseTS(v any) time.Time {
	s, _ := v.(string)
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func num(v any) int {
	n, _ := v.(float64)
	return int(n)
}

func argString(args map[string]json.RawMessage, k string) string {
	var s string
	_ = json.Unmarshal(args[k], &s)
	return s
}

func argInt(args map[string]json.RawMessage, k string, def int) int {
	raw, ok := args[k]
	if !ok {
		return def
	}
	var n int
	if json.Unmarshal(raw, &n) != nil {
		return def
	}
	return n
}

func appendErr(row Row, msg string) {
	if prev, ok := row["last_error"].(string); ok && prev != "" {
		msg = prev + "\n" + msg
	}
	if len(msg) > 4000 {
		msg = msg[len(msg)-4000:]
	}
	row["last_error"] = msg
}

func claimJobs(f *Fake, args map[string]json.RawMessage) (any, error) {
	worker := argString(args, "worker")
	n := argInt(args, "n", 10)
	if n < 1 || n > 100 {
		return nil, fmt.Errorf("n must be between 1 and 100")
	}
	now := f.at()
	var due []Row
	for _, j := range f.rows["jobs"] {
		expired := j["status"] == "running" && parseTS(j["lease_until"]).Before(now)
		queued := j["status"] == "queued" && !parseTS(j["run_after"]).After(now)
		switch {
		case (expired || queued) && num(j["attempts"]) >= num(j["max_attempts"]):
			j["status"] = "failed"
			j["lease_until"] = nil
			if expired {
				appendErr(j, "lease expired after final attempt")
			} else {
				appendErr(j, "no attempts left")
			}
		case expired || queued:
			due = append(due, j)
		}
	}
	slices.SortStableFunc(due, func(a, b Row) int { return parseTS(a["run_after"]).Compare(parseTS(b["run_after"])) })
	out := []Row{}
	for _, j := range due[:min(n, len(due))] {
		j["status"] = "running"
		j["claimed_by"] = worker
		j["claimed_at"] = ts(now)
		j["lease_until"] = ts(now.Add(5 * time.Minute))
		j["attempts"] = float64(num(j["attempts"]) + 1)
		out = append(out, copyRow(j))
	}
	return out, nil
}

func heldJob(f *Fake, args map[string]json.RawMessage) Row {
	id, worker, attempt := argString(args, "job_id"), argString(args, "worker"), argInt(args, "attempt", -1)
	for _, j := range f.rows["jobs"] {
		if j["id"] == id && j["status"] == "running" && j["claimed_by"] == worker && num(j["attempts"]) == attempt {
			return j
		}
	}
	return nil
}

func extendLease(f *Fake, args map[string]json.RawMessage) (any, error) {
	secs := argInt(args, "seconds", 300)
	if secs < 1 || secs > 3600 {
		return nil, fmt.Errorf("seconds must be between 1 and 3600")
	}
	j := heldJob(f, args)
	if j == nil {
		return Row{"applied": false, "lease_until": nil}, nil
	}
	j["lease_until"] = ts(f.at().Add(time.Duration(secs) * time.Second))
	return Row{"applied": true, "lease_until": j["lease_until"]}, nil
}

func finishJob(f *Fake, args map[string]json.RawMessage) (any, error) {
	outcome := argString(args, "outcome")
	if outcome != "done" && outcome != "retry" && outcome != "failed" {
		return nil, fmt.Errorf("outcome must be done, retry or failed")
	}
	retryIn := argInt(args, "retry_in", 60)
	if outcome == "retry" && (retryIn < 0 || retryIn > 86400) {
		return nil, fmt.Errorf("retry_in out of range")
	}
	j := heldJob(f, args)
	if j == nil {
		return Row{"applied": false, "status": nil}, nil
	}
	errMsg, hasErr := "", false
	if raw, ok := args["error"]; ok && string(raw) != "null" {
		_ = json.Unmarshal(raw, &errMsg)
		hasErr = true
	}
	switch {
	case outcome == "done":
		j["status"] = "done"
	case outcome == "retry" && num(j["attempts"]) < num(j["max_attempts"]):
		j["status"] = "queued"
		j["run_after"] = ts(f.at().Add(time.Duration(retryIn) * time.Second))
	default:
		j["status"] = "failed"
	}
	if outcome != "done" && hasErr {
		appendErr(j, errMsg)
	}
	j["lease_until"] = nil
	return Row{"applied": true, "status": j["status"]}, nil
}

package websession

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// devin_quota.go reads the account-level quota the Devin CLI caches locally.
//
// Per-session ACU/credit costs are never emitted over ACP (usage_update and
// turn_stats carry the fields in schema but leave them unset), so the only
// quota signal available to a local client is the CLI's cached
// GetUserStatusResponse written to <cli-dir>/user_status.<digest>.bin on every
// refresh. The payload is protobuf; we decode just the plan_info subtree with
// a minimal wire reader rather than depending on the private .proto.
//
// Field mapping inside GetUserStatusResponse.plan_info (verified against
// cached payloads):
//   f1  PlanInfo    -> f2 string = plan name ("Pro")
//   f14 varint      = daily quota remaining percent
//   f15 varint      = weekly quota remaining percent
//   f17 varint      = daily quota reset time (unix seconds)
//   f18 varint      = weekly quota reset time (unix seconds)

// DevinQuotaStatus is the account-level quota snapshot attached to Devin
// session summaries. Percent fields are 0-100 remaining values.
type DevinQuotaStatus struct {
	PlanName           string `json:"planName"`
	DailyRemainingPct  int64  `json:"dailyRemainingPercent"`
	WeeklyRemainingPct int64  `json:"weeklyRemainingPercent"`
	DailyResetAtUnix   int64  `json:"dailyResetAtUnix"`
	WeeklyResetAtUnix  int64  `json:"weeklyResetAtUnix"`
	FetchedAtUnix      int64  `json:"fetchedAtUnix"`
}

type devinProtoField struct {
	num    int
	wire   int
	varint uint64
	bytes  []byte
}

// devinProtoParse decodes a protobuf buffer into fields, stopping at the first
// malformed entry. Only wire types the status payload uses are needed.
func devinProtoParse(buf []byte) []devinProtoField {
	fields := make([]devinProtoField, 0, 8)
	readVarint := func(pos int) (uint64, int, bool) {
		var result uint64
		var shift uint
		for i := 0; i < 10; i++ {
			if pos >= len(buf) {
				return 0, pos, false
			}
			b := buf[pos]
			pos++
			result |= uint64(b&0x7f) << shift
			if b&0x80 == 0 {
				return result, pos, true
			}
			shift += 7
		}
		return 0, pos, false
	}
	for pos := 0; pos < len(buf); {
		key, next, ok := readVarint(pos)
		if !ok {
			break
		}
		pos = next
		f := devinProtoField{num: int(key >> 3), wire: int(key & 7)}
		switch f.wire {
		case 0:
			v, n, ok := readVarint(pos)
			if !ok {
				return fields
			}
			f.varint, pos = v, n
		case 1:
			if pos+8 > len(buf) {
				return fields
			}
			f.bytes = buf[pos : pos+8]
			pos += 8
		case 2:
			ln, n, ok := readVarint(pos)
			if !ok || n+int(ln) > len(buf) {
				return fields
			}
			f.bytes = buf[n : n+int(ln)]
			pos = n + int(ln)
		case 5:
			if pos+4 > len(buf) {
				return fields
			}
			f.bytes = buf[pos : pos+4]
			pos += 4
		default:
			return fields
		}
		fields = append(fields, f)
	}
	return fields
}

func devinProtoFieldBytes(buf []byte, num int) []byte {
	for _, f := range devinProtoParse(buf) {
		if f.num == num && f.wire == 2 {
			return f.bytes
		}
	}
	return nil
}

func devinProtoVarint(buf []byte, num int) (int64, bool) {
	for _, f := range devinProtoParse(buf) {
		if f.num == num && f.wire == 0 {
			return int64(f.varint), true
		}
	}
	return 0, false
}

// decodeDevinUserStatusPayload extracts the plan quota from a cached
// GetUserStatusResponse. Returns nil when the payload does not look like a
// valid status response, so schema drift degrades to "no quota" instead of
// nonsense numbers.
func decodeDevinUserStatusPayload(payload []byte) *DevinQuotaStatus {
	planInfo := devinProtoFieldBytes(payload, 13)
	if len(planInfo) == 0 {
		return nil
	}
	daily, hasDaily := devinProtoVarint(planInfo, 14)
	weekly, hasWeekly := devinProtoVarint(planInfo, 15)
	if (!hasDaily || daily < 0 || daily > 100) && (!hasWeekly || weekly < 0 || weekly > 100) {
		return nil
	}
	quota := &DevinQuotaStatus{}
	if hasDaily && daily >= 0 && daily <= 100 {
		quota.DailyRemainingPct = daily
	}
	if hasWeekly && weekly >= 0 && weekly <= 100 {
		quota.WeeklyRemainingPct = weekly
	}
	if v, ok := devinProtoVarint(planInfo, 17); ok {
		quota.DailyResetAtUnix = v
	}
	if v, ok := devinProtoVarint(planInfo, 18); ok {
		quota.WeeklyResetAtUnix = v
	}
	if plan := devinProtoFieldBytes(planInfo, 1); len(plan) > 0 {
		for _, f := range devinProtoParse(plan) {
			if f.num == 2 && f.wire == 2 {
				quota.PlanName = string(f.bytes)
				break
			}
		}
	}
	return quota
}

// devinUserStatusDirs returns candidate directories holding the CLI's cached
// user_status files: the CLI binary's sibling layout (<install>/cli) first,
// then platform data dirs.
func devinUserStatusDirs(devinPath string) []string {
	var dirs []string
	if resolved, err := exec.LookPath(devinPath); err == nil {
		if abs, err := filepath.Abs(resolved); err == nil {
			resolved = abs
		}
		dir := filepath.Dir(resolved)
		if filepath.Base(dir) == "bin" {
			dirs = append(dirs, filepath.Dir(dir))
		}
		dirs = append(dirs, dir)
	}
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			dirs = append(dirs, filepath.Join(local, "devin", "cli"))
		}
	} else if home, err := os.UserHomeDir(); err == nil {
		if runtime.GOOS == "darwin" {
			dirs = append(dirs, filepath.Join(home, "Library", "Application Support", "devin", "cli"))
		}
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			dirs = append(dirs, filepath.Join(xdg, "devin", "cli"))
		}
		dirs = append(dirs, filepath.Join(home, ".local", "share", "devin", "cli"))
	}
	return dirs
}

// newestDevinUserStatusFile picks the most recently written user_status.*.bin
// across the candidate directories.
func newestDevinUserStatusFile(dirs []string) string {
	var best string
	var bestMod time.Time
	for _, dir := range dirs {
		matches, err := filepath.Glob(filepath.Join(dir, "user_status.*.bin"))
		if err != nil {
			continue
		}
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil || info.IsDir() {
				continue
			}
			if best == "" || info.ModTime().After(bestMod) {
				best, bestMod = match, info.ModTime()
			}
		}
	}
	return best
}

// readDevinUserStatusQuota parses one cached status file into a quota
// snapshot, or nil when unreadable/invalid.
func readDevinUserStatusQuota(path string) *DevinQuotaStatus {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var envelope struct {
		FetchedAt int64  `json:"fetched_at_secs"`
		Payload   string `json:"payload"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.Payload == "" {
		return nil
	}
	payload, err := base64.StdEncoding.DecodeString(envelope.Payload)
	if err != nil {
		return nil
	}
	quota := decodeDevinUserStatusPayload(payload)
	if quota == nil {
		return nil
	}
	quota.FetchedAtUnix = envelope.FetchedAt
	return quota
}

// devinPlanQuota returns the cached account quota, re-reading the newest
// user_status file only when its mtime changed. Nil when the cache is absent
// or undecodable.
func (m *Manager) devinPlanQuota() *DevinQuotaStatus {
	m.devinQuotaMu.Lock()
	defer m.devinQuotaMu.Unlock()
	path := newestDevinUserStatusFile(devinUserStatusDirs(m.cfg.DevinPath))
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if m.devinQuotaCache != nil && m.devinQuotaCache.path == path &&
		m.devinQuotaCache.modTime.Equal(info.ModTime()) {
		return m.devinQuotaCache.quota
	}
	quota := readDevinUserStatusQuota(path)
	m.devinQuotaCache = &devinQuotaCacheEntry{path: path, modTime: info.ModTime(), quota: quota}
	return quota
}

type devinQuotaCacheEntry struct {
	path    string
	modTime time.Time
	quota   *DevinQuotaStatus
}

package websession

import (
	"testing"
)

func devinTestVarint(buf []byte, v uint64) []byte {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v == 0 {
			return append(buf, b)
		}
		buf = append(buf, b|0x80)
	}
}

func devinTestVarintField(buf []byte, num int, v uint64) []byte {
	buf = devinTestVarint(buf, uint64(num<<3))
	return devinTestVarint(buf, v)
}

func devinTestLenField(buf []byte, num int, payload []byte) []byte {
	buf = devinTestVarint(buf, uint64(num<<3)|2)
	buf = devinTestVarint(buf, uint64(len(payload)))
	return append(buf, payload...)
}

func TestDecodeDevinUserStatusPayload(t *testing.T) {
	planInfo := devinTestLenField(nil, 2, []byte("Pro"))
	plan := devinTestLenField(nil, 1, planInfo)
	plan = devinTestVarintField(plan, 14, 95)
	plan = devinTestVarintField(plan, 15, 87)
	plan = devinTestVarintField(plan, 17, 1790841600)
	plan = devinTestVarintField(plan, 18, 1791187200)
	payload := devinTestLenField(nil, 13, plan)

	quota := decodeDevinUserStatusPayload(payload)
	if quota == nil {
		t.Fatal("expected quota, got nil")
	}
	if quota.PlanName != "Pro" {
		t.Fatalf("plan name = %q, want Pro", quota.PlanName)
	}
	if quota.DailyRemainingPct != 95 {
		t.Fatalf("daily = %d, want 95", quota.DailyRemainingPct)
	}
	if quota.WeeklyRemainingPct != 87 {
		t.Fatalf("weekly = %d, want 87", quota.WeeklyRemainingPct)
	}
	if quota.DailyResetAtUnix != 1790841600 {
		t.Fatalf("daily reset = %d", quota.DailyResetAtUnix)
	}
	if quota.WeeklyResetAtUnix != 1791187200 {
		t.Fatalf("weekly reset = %d", quota.WeeklyResetAtUnix)
	}
}

func TestDecodeDevinUserStatusPayloadRejectsInvalid(t *testing.T) {
	if quota := decodeDevinUserStatusPayload(nil); quota != nil {
		t.Fatal("expected nil for empty payload")
	}
	// plan_info present but quota percents out of range.
	plan := devinTestVarintField(nil, 14, 250)
	payload := devinTestLenField(nil, 13, plan)
	if quota := decodeDevinUserStatusPayload(payload); quota != nil {
		t.Fatal("expected nil for out-of-range percent")
	}
}
